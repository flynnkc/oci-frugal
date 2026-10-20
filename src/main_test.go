package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/configuration"
	"github.com/flynnkc/oci-frugal/src/pkg/controller"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeController struct {
	region string
	// Regions run concurrently, so counters must be safe to read from callbacks.
	plans, executes, closes atomic.Int32
	planErr, executeErr     error
	check                   func()
}

func (c *fakeController) Plan(context.Context) (controller.Result, error) {
	c.plans.Add(1)
	r := controller.Result{Region: c.region, DryRun: true}
	if c.planErr != nil {
		r.Error = c.planErr.Error()
	}
	return r, c.planErr
}
func (c *fakeController) Execute(
	_ context.Context,
	r controller.Result,
) (controller.Result, error) {
	if c.check != nil {
		c.check()
	}
	c.executes.Add(1)
	r.DryRun = false
	return r, c.executeErr
}
func (c *fakeController) Close()  { c.closes.Add(1) }
func noEnv(string) (string, bool) { return "", false }
func TestRunPhasesAndCleanup(t *testing.T) {
	for _, scenario := range []string{
		"dry_run",
		"execute",
		"plan_failure",
		"execute_failure",
		"construction_failure",
	} {
		t.Run(scenario, func(t *testing.T) {
			cfg, err := configuration.NewConfiguration(configuration.ConfigurationOpts{
				Timezone: common.String("Pacific/Auckland"),
				DryRun:   scenario == "dry_run",
				Shutdown: common.String("hard"),
			})
			require.NoError(t, err)
			first := &fakeController{region: "one"}
			second := &fakeController{region: "two"}
			if scenario == "plan_failure" {
				second.planErr = errors.New("invalid schedule")
			}
			if scenario == "execute_failure" {
				second.executeErr = errors.New("OCI denied")
			}
			first.check = func() {
				// This callback runs in a worker; require would call FailNow on that goroutine.
				assert.Equal(
					t,
					int32(1),
					second.plans.Load(),
					"execution began before all regions planned",
				)
			}
			deps := dependencies{regions: func(
				context.Context,
				*configuration.Configuration,
			) ([]string, error) {
				return []string{"one", "two"}, nil
			}, controller: func(opts controller.ControllerOpts) (controller.Controller, error) {
				assert.Equal(t, "hard", opts.Shutdown)
				if *opts.Region == "one" {
					return first, nil
				}
				if scenario == "construction_failure" {
					return nil, errors.New("invalid provider")
				}
				return second, nil
			}}
			r, err := run(
				context.Background(),
				cfg,
				deps,
			)
			assert.Equal(t, cfg.DryRun(), r.DryRun)
			assert.Equal(t, cfg.Shutdown(), r.Shutdown)
			failed := strings.HasSuffix(scenario, "failure")
			if failed {
				assert.Error(t, err)
				assert.NotEmpty(t, r.Error)
			} else {
				assert.NoError(t, err)
				assert.Empty(t, r.Error)
			}
			assert.Equal(t, int32(1), first.closes.Load(), "first controller leaked")
			if scenario != "construction_failure" {
				assert.Equal(t, int32(1), second.closes.Load(), "second controller leaked")
			}
			expected := int32(1)
			// A failure during planning must block writes in the healthy region too.
			if scenario == "dry_run" ||
				scenario == "plan_failure" ||
				scenario == "construction_failure" {
				expected = 0
			}
			assert.Equal(t, expected, first.executes.Load(), "first controller execution count")
			assert.Equal(
				t,
				expected,
				second.executes.Load(),
				"second controller execution count",
			)
		})
	}
}
func TestCommandExitCodesAndJSON(t *testing.T) {
	for _, tt := range []struct {
		args []string
		code int
	}{
		{[]string{"-help"}, 0},
		{[]string{"-action", "up"}, 2},
		{[]string{"-shutdown", "invalid"}, 2},
		{[]string{"-timeout", "0"}, 2},
		{[]string{"-timeout", "-1s"}, 2},
		{[]string{"-timeout", "invalid"}, 2},
		{[]string{"-shutdown="}, 2},
		{[]string{"-config-file", ""}, 2},
		{[]string{"-dry-run", "-region", "test"}, 1},
	} {
		var out, errOut bytes.Buffer
		deps := dependencies{controller: func(
			controller.ControllerOpts,
		) (controller.Controller, error) {
			return nil, errors.New("mock provider error")
		}}
		code := command(context.Background(), tt.args, noEnv, &out, &errOut, deps)
		assert.Equal(t, tt.code, code, "args: %v; output: %s", tt.args, out.String())
		if tt.code != 0 {
			var result map[string]any
			require.NoError(
				t,
				json.Unmarshal(out.Bytes(), &result),
				"output not JSON: %s",
				out.String(),
			)
		}
	}
}

func TestRunAppliesConfiguredTimeout(t *testing.T) {
	timeout := 10 * time.Millisecond
	cfg, err := configuration.NewConfiguration(configuration.ConfigurationOpts{
		Timeout: &timeout,
	})
	require.NoError(t, err)
	// Bound the test itself, but require run to apply the shorter configured limit.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	deps := dependencies{
		regions: func(
			ctx context.Context,
			_ *configuration.Configuration,
		) ([]string, error) {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok)
			assert.LessOrEqual(t, time.Until(deadline), timeout)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	_, err = run(ctx, cfg, deps)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestFlagPrecedence(t *testing.T) {
	env := func(k string) (string, bool) {
		m := map[string]string{
			"FRUGAL_ACTION_TYPE": "off",
			"FRUGAL_DRY_RUN":     "true",
			"FRUGAL_TIMEOUT":     "2m",
			"FRUGAL_SHUTDOWN":    "hard",
		}
		v, ok := m[k]
		return v, ok
	}
	opts, err := parseOptions([]string{"-action", "on", "-dry-run=false"}, env, &bytes.Buffer{})
	require.NoError(t, err)
	require.NotNil(t, opts.Action)
	assert.Equal(t, "on", *opts.Action)
	assert.False(t, opts.DryRun)
	require.NotNil(t, opts.Shutdown)
	assert.Equal(t, "hard", *opts.Shutdown)
	require.NotNil(t, opts.Timeout)
	assert.Equal(t, 2*time.Minute, *opts.Timeout)
}

func TestOptionalStringFlagsPreserveUnsetAndEmptyValues(t *testing.T) {
	opts, err := parseOptions(nil, noEnv, &bytes.Buffer{})
	require.NoError(t, err)
	assert.Nil(t, opts.ConfigProfile, "unset values must retain configuration defaults")
	assert.Nil(t, opts.Shutdown)
	assert.Nil(t, opts.Timeout)

	env := func(key string) (string, bool) {
		if key == "FRUGAL_PROFILE" {
			return "DEFAULT", true
		}
		if key == "FRUGAL_TAG_NAMESPACE" {
			return "", true
		}
		return "", false
	}
	// An explicit empty flag overrides the environment; an empty environment value
	// also stays explicit so configuration validation can distinguish it from unset.
	opts, err = parseOptions([]string{"-profile="}, env, &bytes.Buffer{})
	require.NoError(t, err)
	require.NotNil(t, opts.ConfigProfile)
	assert.Empty(t, *opts.ConfigProfile)
	require.NotNil(t, opts.TagNamespace)
	assert.Empty(t, *opts.TagNamespace)
	assert.Nil(t, opts.Timezone)
}
