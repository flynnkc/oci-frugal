package ociapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCancellationAndCloseBlockRequests(t *testing.T) {
	l := NewLimiter()
	defer l.Close()
	calls := 0
	fn := func(ctx context.Context) (int, error) {
		calls++
		_, ok := ctx.Deadline()
		assert.True(t, ok, "missing request deadline")
		return 42, nil
	}
	v, err := Call(context.Background(), l, fn)
	require.NoError(t, err)
	assert.Equal(t, 42, v)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Call(ctx, l, fn)
	assert.ErrorIs(t, err, context.Canceled)
	// Close while tokens remain buffered: shutdown must reject even an immediate acquisition.
	l.Close()
	_, err = Call(context.Background(), l, fn)
	assert.ErrorContains(t, err, "limiter closed")
	assert.Equal(t, 1, calls)
}
