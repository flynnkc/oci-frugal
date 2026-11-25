// Package ociapi bounds and rate limits OCI requests, including reads.
package ociapi

import (
	"context"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	tokenpool "github.com/flynnkc/token-pool"
)

type Limiter interface{ Acquire(context.Context) bool }

// TokenLimiter rejects requests after shutdown, including callers that were
// waiting when the underlying pool closed.
type TokenLimiter struct {
	pool   *tokenpool.TokenPool
	closed atomic.Bool
	done   chan struct{}
}

const DefaultRate = 2.0
const DefaultBurst = 4

// ValidateLimits rejects non-finite rates and intervals outside timer precision.
func ValidateLimits(rate float64, burst int) error {
	interval := float64(time.Second) / rate
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 || interval < 1 || interval >= float64(math.MaxInt64) || burst < 1 {
		return fmt.Errorf("rate must be finite and positive with a representable refill interval; burst must be positive")
	}
	return nil
}

func NewLimiter() *TokenLimiter {
	l, _ := NewConfiguredLimiter(DefaultRate, DefaultBurst)
	return l
}

func NewConfiguredLimiter(rate float64, burst int) (*TokenLimiter, error) {
	if err := ValidateLimits(rate, burst); err != nil {
		return nil, err
	}
	return &TokenLimiter{done: make(chan struct{}), pool: tokenpool.NewTokenPool(burst, 1, time.Duration(math.Ceil(float64(time.Second)/rate)))}, nil
}
func (l *TokenLimiter) Acquire(ctx context.Context) bool {
	if l.closed.Load() || ctx.Err() != nil {
		return false
	}
	// Closing the pool can unblock a waiting caller; do not treat that as a token grant.
	return l.pool.Acquire(ctx) && !l.closed.Load() && ctx.Err() == nil
}
func (l *TokenLimiter) Close() {
	if !l.closed.Swap(true) {
		close(l.done)
		l.pool.Close()
	}
}

func Call[T any](
	ctx context.Context,
	fn func(context.Context) (T, error),
) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	// The whole SDK operation, including HTTP limiter waits and SDK retries, is bounded.
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return fn(context.WithValue(requestCtx, attemptKey{}, new(atomic.Uint64)))
}
