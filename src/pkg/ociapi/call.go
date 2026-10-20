// Package ociapi bounds and rate limits OCI requests, including reads.
package ociapi

import (
	"context"
	"fmt"
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
}

func NewLimiter() *TokenLimiter {
	return &TokenLimiter{pool: tokenpool.NewTokenPool(8, 8, 15*time.Second)}
}
func (l *TokenLimiter) Acquire(ctx context.Context) bool {
	if l.closed.Load() {
		return false
	}
	// Closing the pool can unblock a waiting caller; do not treat that as a token grant.
	return l.pool.Acquire(ctx) && !l.closed.Load()
}
func (l *TokenLimiter) Close() { l.closed.Store(true); l.pool.Close() }

func Call[T any](
	ctx context.Context,
	limiter Limiter,
	fn func(context.Context) (T, error),
) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if limiter == nil {
		return zero, fmt.Errorf("missing OCI request limiter")
	}
	if !limiter.Acquire(ctx) {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		return zero, fmt.Errorf("OCI request limiter closed")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	// Queueing uses the run's deadline; the SDK call gets its own bounded request window.
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return fn(requestCtx)
}
