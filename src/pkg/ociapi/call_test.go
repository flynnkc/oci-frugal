package ociapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallCancellationAndDeadline(t *testing.T) {
	calls := 0
	fn := func(ctx context.Context) (int, error) {
		calls++
		_, ok := ctx.Deadline()
		assert.True(t, ok, "missing request deadline")
		return 42, nil
	}
	v, err := Call(context.Background(), fn)
	require.NoError(t, err)
	assert.Equal(t, 42, v)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Call(ctx, fn)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls)
}
