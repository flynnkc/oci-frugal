package configuration

import (
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvalidConfiguration(t *testing.T) {
	zeroTimeout := time.Duration(0)
	negativeTimeout := -time.Second
	for _, opts := range []ConfigurationOpts{
		{Action: common.String("up")},
		{LogLevel: common.String("verbose")},
		{TagNamespace: common.String("")},
		{Timezone: common.String("not/a/timezone")},
		{ConfigFile: common.String("")},
		{ConfigFile: common.String(""), Principal: common.String(APIKEY)},
		{Principal: common.String("unknown")},
		{Shutdown: common.String("invalid")},
		{Shutdown: common.String("")},
		{Timeout: &zeroTimeout},
		{Timeout: &negativeTimeout},
	} {
		_, err := NewConfiguration(opts)
		assert.Error(t, err, "accepted invalid options: %+v", opts)
	}
}

func TestExecutionDefaults(t *testing.T) {
	cfg, err := NewConfiguration(ConfigurationOpts{})
	require.NoError(t, err)
	assert.False(t, cfg.DryRun())
	assert.Equal(t, "soft", cfg.Shutdown())
	assert.Equal(t, 10*time.Minute, cfg.Timeout())
}

func TestExplicitExecutionConfiguration(t *testing.T) {
	timeout := 45 * time.Second
	cfg, err := NewConfiguration(ConfigurationOpts{
		DryRun:   true,
		Shutdown: common.String("hard"),
		Timeout:  &timeout,
	})
	require.NoError(t, err)
	assert.True(t, cfg.DryRun())
	assert.Equal(t, "hard", cfg.Shutdown())
	assert.Equal(t, timeout, cfg.Timeout())
}
