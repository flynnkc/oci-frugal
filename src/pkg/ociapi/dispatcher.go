package ociapi

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
)

type attemptKey struct{}

// Budget shares admission control, 429 cooldown and metrics across a region.
// It never retries requests: the SDK retains ownership of retry eligibility.
type Budget struct {
	limiter                      *TokenLimiter
	log                          *slog.Logger
	mu                           sync.Mutex
	until                        time.Time
	strikes                      uint
	attempts, retries, throttled atomic.Uint64
	waitNanos                    atomic.Int64
}

func NewBudget(limiter *TokenLimiter, log *slog.Logger) *Budget {
	return &Budget{limiter: limiter, log: log}
}

func (b *Budget) Acquire(ctx context.Context) bool {
	started := time.Now()
	defer func() { b.waitNanos.Add(int64(time.Since(started))) }()
	for {
		if ctx.Err() != nil || b.limiter.closed.Load() {
			return false
		}
		b.mu.Lock()
		wait := time.Until(b.until)
		b.mu.Unlock()
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return false
			case <-b.limiter.done:
				timer.Stop()
				return false
			case <-timer.C:
			}
		}
		if !b.limiter.Acquire(ctx) {
			return false
		}
		// A concurrent request may have extended the regional cooldown while waiting.
		b.mu.Lock()
		cooling := time.Now().Before(b.until)
		b.mu.Unlock()
		if !cooling {
			return ctx.Err() == nil
		}
	}
}

func (b *Budget) observe(status int) {
	if status != http.StatusTooManyRequests {
		return
	}
	b.throttled.Add(1)
	b.mu.Lock()
	// A quiet two-minute period starts a new backoff sequence. Equal jitter
	// yields 2–4s initially, growing to 30–60s; all waiters share the cooldown.
	if time.Since(b.until) > 2*time.Minute {
		b.strikes = 0
	}
	if b.strikes < 5 {
		b.strikes++
	}
	ceiling := min(60*time.Second, (2*time.Second)<<b.strikes)
	delay := ceiling/2 + time.Duration(rand.Float64()*float64(ceiling/2))
	until := time.Now().Add(delay)
	if until.After(b.until) {
		b.until = until
	}
	b.limiter.pool.Drain()
	b.mu.Unlock()
	if b.log != nil {
		b.log.Warn("OCI throttled request; regional cooldown applied", "cooldown", delay)
	}
}

func (b *Budget) Summary() {
	if b.log != nil {
		b.log.Info("Regional API usage", "http_attempts", b.attempts.Load(), "retries", b.retries.Load(), "throttled", b.throttled.Load(), "limiter_wait", time.Duration(b.waitNanos.Load()))
	}
}

// Dispatcher wraps the SDK's existing HTTP dispatcher, preserving its TLS,
// proxy and transport settings. Each SDK retry returns through Do and is gated.
type Dispatcher struct {
	next    common.HTTPRequestDispatcher
	limiter Limiter
	log     *slog.Logger
}

func NewDispatcher(next common.HTTPRequestDispatcher, limiter Limiter, log *slog.Logger) *Dispatcher {
	return &Dispatcher{next: next, limiter: limiter, log: log}
}

func (d *Dispatcher) Do(req *http.Request) (*http.Response, error) {
	started := time.Now()
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	if !d.limiter.Acquire(req.Context()) {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("OCI request limiter closed")
	}
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	attempt := uint64(1)
	if counter, ok := req.Context().Value(attemptKey{}).(*atomic.Uint64); ok {
		attempt = counter.Add(1)
	}
	if b, ok := d.limiter.(*Budget); ok {
		b.attempts.Add(1)
		if attempt > 1 {
			b.retries.Add(1)
		}
	}
	if d.log != nil {
		d.log.Debug("OCI HTTP attempt started", "method", req.Method, "attempt", attempt, "limiter_wait", time.Since(started))
	}
	resp, err := d.next.Do(req)
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	if b, ok := d.limiter.(*Budget); ok {
		b.observe(status)
	}
	if d.log != nil {
		d.log.Debug("OCI HTTP attempt finished", "attempt", attempt, "status", status, "elapsed", time.Since(started))
	}
	return resp, err
}
