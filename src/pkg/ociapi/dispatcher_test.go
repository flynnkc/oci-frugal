package ociapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dispatchFunc func(*http.Request) (*http.Response, error)

func (f dispatchFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestRateBurstAndClose(t *testing.T) {
	l, err := NewConfiguredLimiter(20, 2)
	require.NoError(t, err)
	defer l.Close()
	require.True(t, l.Acquire(context.Background()))
	require.True(t, l.Acquire(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	require.False(t, l.Acquire(ctx), "burst must not refill immediately")
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	require.True(t, l.Acquire(ctx2), "one token should refill without a batch")
	l.Close()
	require.False(t, l.Acquire(context.Background()))
}

func TestInvalidLimits(t *testing.T) {
	for _, r := range []float64{0, -1, math.NaN(), math.Inf(1), 1e12, 1e-20} {
		_, err := NewConfiguredLimiter(r, 4)
		require.Error(t, err)
	}
	_, err := NewConfiguredLimiter(2, 0)
	require.Error(t, err)
}

func TestSDKRetryConsumesAnotherToken(t *testing.T) {
	l, err := NewConfiguredLimiter(0.01, 2)
	require.NoError(t, err)
	defer l.Close()
	b := NewBudget(l, nil)
	var calls int
	d := NewDispatcher(dispatchFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		status := http.StatusOK
		if calls == 1 {
			status = http.StatusServiceUnavailable
		}
		return &http.Response{StatusCode: status}, nil
	}), b, nil)
	// Exercise the actual SDK retry loop without network I/O or changing production
	// retry policy. A retry must enter the same HTTP gate as the initial attempt.
	policy := common.RetryPolicy{
		MaximumNumberAttempts: 2,
		ShouldRetryOperation:  func(r common.OCIOperationResponse) bool { return r.Error != nil },
		NextDuration:          func(common.OCIOperationResponse) time.Duration { return 0 },
	}

	_, err = Call(context.Background(), func(ctx context.Context) (common.OCIResponse, error) {
		return common.Retry(ctx, core.GetInstanceRequest{}, func(ctx context.Context, _ common.OCIRequest, _ *common.OCIReadSeekCloser, _ map[string]string) (common.OCIResponse, error) {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid", nil)
			resp, err := d.Do(req)
			if err == nil && resp.StatusCode != http.StatusOK {
				err = errors.New("synthetic service failure")
			}
			return core.GetInstanceResponse{RawResponse: resp}, err
		}, policy)
	})
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.Equal(t, uint64(2), b.attempts.Load())
	assert.Equal(t, uint64(1), b.retries.Load())
	assert.False(t, l.pool.TryToken(), "retry must consume the second burst token")
}

func TestThrottleCooldownIsSharedAndCancelable(t *testing.T) {
	l := NewLimiter()
	defer l.Close()
	b := NewBudget(l, nil)
	var calls atomic.Int32
	d := NewDispatcher(dispatchFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 429}, nil
	}), b, nil)
	req, _ := http.NewRequest(http.MethodPost, "https://example.invalid", nil)
	resp, err := d.Do(req)
	require.NoError(t, err)
	require.Equal(t, 429, resp.StatusCode)
	require.Equal(t, int32(1), calls.Load(), "dispatcher must not retry writes")
	b.mu.Lock()
	delay := time.Until(b.until)
	b.mu.Unlock()
	assert.Greater(t, delay, time.Second)
	assert.LessOrEqual(t, delay, 4*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)
	_, err = d.Do(req)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, uint64(1), b.throttled.Load())
	// Close must unblock cooldown waiters, not leave them sleeping for up to 60s.
	stopped := make(chan bool, 1)
	go func() { stopped <- b.Acquire(context.Background()) }()
	l.Close()
	select {
	case acquired := <-stopped:
		assert.False(t, acquired)
	case <-time.After(time.Second):
		t.Fatal("close did not unblock cooldown")
	}
}

func TestSharedBudgetAndIndependentRegions(t *testing.T) {
	first, err := NewConfiguredLimiter(0.01, 2)
	require.NoError(t, err)
	defer first.Close()
	second, err := NewConfiguredLimiter(0.01, 2)
	require.NoError(t, err)
	defer second.Close()
	b := NewBudget(first, nil)
	var calls atomic.Int32
	next := dispatchFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200}, nil
	})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid", nil)
			_, _ = NewDispatcher(next, b, nil).Do(req)
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(2), calls.Load(), "all service dispatchers must share one burst")
	assert.True(t, second.Acquire(context.Background()), "another region must have its own budget")
	first.Close()
	req, _ := http.NewRequest(http.MethodPost, "https://example.invalid", nil)
	_, err = NewDispatcher(next, b, nil).Do(req)
	assert.ErrorContains(t, err, "limiter closed")
	assert.Equal(t, int32(2), calls.Load())
}

func TestCooldownBackoffIsBounded(t *testing.T) {
	l := NewLimiter()
	defer l.Close()
	b := NewBudget(l, nil)
	for range 20 {
		b.observe(429)
	}
	b.mu.Lock()
	delay := time.Until(b.until)
	b.mu.Unlock()
	assert.Greater(t, delay, 29*time.Second)
	assert.LessOrEqual(t, delay, 60*time.Second)
}
