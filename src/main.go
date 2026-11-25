package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/configuration"
	"github.com/flynnkc/oci-frugal/src/pkg/controller"
	"github.com/flynnkc/oci-frugal/src/pkg/id"
	"github.com/flynnkc/oci-frugal/src/pkg/scheduler"
)

type report struct {
	Shutdown   string              `json:"shutdown"`
	DryRun     bool                `json:"dry_run"`
	Phase      string              `json:"phase"`
	StartedAt  time.Time           `json:"started_at"`
	FinishedAt time.Time           `json:"finished_at"`
	Regions    []controller.Result `json:"regions"`
	Error      string              `json:"error,omitempty"`
}

type dependencies struct {
	regions    func(context.Context, *configuration.Configuration) ([]string, error)
	controller func(controller.ControllerOpts) (controller.Controller, error)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := command(ctx, os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr, defaultDependencies())
	stop()
	os.Exit(code)
}

func command(
	ctx context.Context,
	args []string,
	env func(string) (string, bool),
	out, errOut io.Writer,
	deps dependencies,
) int {
	opts, err := parseOptions(args, env, errOut)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		writeCommandError(out, errOut, err)
		return 2
	}
	opts.LogWriter = errOut
	cfg, err := configuration.NewConfiguration(opts)
	if err != nil {
		writeCommandError(out, errOut, err)
		return 2
	}
	result, err := run(ctx, cfg, deps)
	if outputErr := writeJSON(out, result); outputErr != nil {
		fmt.Fprintln(errOut, outputErr)
		return 1
	}
	if err != nil {
		return 1
	}
	return 0
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeCommandError(out, errOut io.Writer, commandErr error) {
	errorResponse := map[string]string{"error": commandErr.Error()}
	if outputErr := writeJSON(out, errorResponse); outputErr != nil {
		// Use diagnostics if writing the structured error to stdout fails.
		fmt.Fprintln(errOut, outputErr)
	}
}

func defaultDependencies() dependencies {
	return dependencies{
		regions: func(ctx context.Context, cfg *configuration.Configuration) ([]string, error) {
			client, err := id.NewIdentityClient(cfg.Provider())
			if err != nil {
				return nil, err
			}
			return client.GetRegions(ctx)
		},
		controller: func(opts controller.ControllerOpts) (controller.Controller, error) {
			return controller.NewTagController(opts)
		},
	}
}

func run(
	ctx context.Context,
	cfg *configuration.Configuration,
	deps dependencies,
) (report, error) {
	// Apply the validated execution limit for every caller, including future API jobs.
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout())
	defer cancel()
	r := report{
		Shutdown:  cfg.Shutdown(),
		DryRun:    cfg.DryRun(),
		Phase:     "planning",
		StartedAt: time.Now().UTC(),
		Regions:   make([]controller.Result, 0),
	}
	finish := func(err error) (report, error) {
		r.FinishedAt = time.Now().UTC()
		if err != nil {
			r.Error = err.Error()
		}
		return r, err
	}
	log := cfg.MakeLog()
	log.Info("Run started", "dry_run", cfg.DryRun(), "timeout", cfg.Timeout(), "timezone", cfg.Timezone().String())
	stopProgress := startProgress(ctx, log, 15*time.Second)
	defer stopProgress()
	defer func() {
		if r.Error != "" {
			log.Error("Run failed", "phase", r.Phase, "error", r.Error)
		} else {
			log.Info("Run finished", "phase", r.Phase, "elapsed", time.Since(r.StartedAt))
		}
	}()
	regions := []string{*cfg.Region()}
	if regions[0] == "" {
		var err error
		log.Info("Discovering subscribed regions")
		regions, err = deps.regions(ctx, cfg)
		if err != nil {
			return finish(err)
		}
	}
	if len(regions) == 0 {
		return finish(fmt.Errorf("no subscribed regions"))
	}
	log.Info("Planning regions", "count", len(regions))
	// Freeze one instant for all regions, interpreted in the selected timezone.
	sch := scheduler.NewAnykeyNLSchedulerAt(r.StartedAt, cfg.Timezone())
	controllers := make([]controller.Controller, len(regions))
	defer func() {
		for _, c := range controllers {
			if c != nil {
				c.Close()
			}
		}
	}()
	r.Regions = make([]controller.Result, len(regions))
	errs := make([]error, len(regions))

	var wg sync.WaitGroup
	for i, region := range regions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Info("Region planning started", "region", region)
			c, err := deps.controller(controller.ControllerOpts{
				ConfigurationProvider: cfg.Provider(),
				TagNamespace:          cfg.TagNamespace(),
				Region:                &region,
				Scheduler:             sch,
				SupportedActions:      *cfg.Action(),
				LogFunc:               cfg.MakeLog,
				Shutdown:              cfg.Shutdown(),
			})
			if err != nil {
				errs[i] = fmt.Errorf("%s: %w", region, err)
				r.Regions[i] = controller.Result{
					Region: region,
					DryRun: true,
					Error:  err.Error(),
				}
				return
			}
			controllers[i] = c
			r.Regions[i], err = c.Plan(ctx)
			log.Info("Region planning finished", "region", region, "resources", len(r.Regions[i].Resources), "error", err)
			if err != nil {
				errs[i] = fmt.Errorf("%s: %w", region, err)
			}
		}()
	}
	// Join every region's planning work before allowing any region to submit writes.
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		r.Phase = "blocked"
		return finish(err)
	}
	if err := ctx.Err(); err != nil {
		r.Phase = "blocked"
		return finish(err)
	}
	if cfg.DryRun() {
		r.Phase = "dry_run"
		return finish(nil)
	}
	r.Phase = "execution"
	log.Info("Submitting planned operations")
	for i, c := range controllers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			log.Info("Region execution started", "region", regions[i])
			r.Regions[i], err = c.Execute(ctx, r.Regions[i])
			log.Info("Region execution finished", "region", regions[i], "error", err)
			if err != nil {
				errs[i] = fmt.Errorf("%s: %w", regions[i], err)
			}
		}()
	}
	wg.Wait()
	return finish(errors.Join(errs...))
}

// parseOptions uses a fresh FlagSet with explicit arguments instead of the global
// flag.Parse so tests can supply arguments, environment values, and diagnostic
// output without changing process globals. ContinueOnError returns parsing errors
// to command, which controls JSON error output and exit codes without exiting here.
// Execution defaults and semantic validation are applied by NewConfiguration.
func parseOptions(
	args []string,
	env func(string) (string, bool),
	errOut io.Writer,
) (configuration.ConfigurationOpts, error) {
	var opts configuration.ConfigurationOpts
	fs := flag.NewFlagSet("oci-frugal", flag.ContinueOnError)
	fs.SetOutput(errOut)
	// Keep optional strings nil when neither source supplies a value. Loading the
	// environment first lets flag parsing override it, including with an empty string.
	stringFlag := func(target **string, name, key, usage string) {
		if value, ok := env("FRUGAL_" + key); ok {
			*target = &value
		}
		fs.Func(name, usage, func(value string) error {
			*target = &value
			return nil
		})
	}

	stringFlag(&opts.Timezone, "tz", "TIMEZONE", "Timezone, e.g. America/New_York")
	stringFlag(&opts.Action, "action", "ACTION_TYPE", "Power filter: all, on, off")
	stringFlag(
		&opts.Principal, "auth", "AUTH_TYPE",
		"Authentication: api_key, instance_principal, resource_principal, workload_principal",
	)
	stringFlag(
		&opts.Region, "region", "REGION",
		"Region; defaults to all subscribed regions",
	)
	stringFlag(&opts.ConfigFile, "config-file", "FILE", "OCI configuration file")
	stringFlag(&opts.ConfigProfile, "profile", "PROFILE", "OCI configuration profile")
	stringFlag(
		&opts.KeyPassword, "key-pass", "KEY_PASS",
		"Private key password (prefer FRUGAL_KEY_PASS)",
	)
	stringFlag(&opts.LogLevel, "v", "LOG_LEVEL", "Log level: debug, info, warn, error")
	stringFlag(&opts.TagNamespace, "tag", "TAG_NAMESPACE", "Schedule tag namespace")

	var envErr error
	if v, ok := env("FRUGAL_DRY_RUN"); ok {
		opts.DryRun, envErr = strconv.ParseBool(v)
	}
	if envErr != nil {
		return opts, fmt.Errorf("FRUGAL_DRY_RUN: %w", envErr)
	}
	stringFlag(
		&opts.Shutdown, "shutdown", "SHUTDOWN",
		fmt.Sprintf(
			"Compute shutdown policy: soft or hard (default %s)",
			configuration.DEFAULT_SHUTDOWN,
		),
	)
	if value, ok := env("FRUGAL_TIMEOUT"); ok {
		timeout, err := time.ParseDuration(value)
		if err != nil {
			return opts, fmt.Errorf("FRUGAL_TIMEOUT: %w", err)
		}
		opts.Timeout = &timeout
	}
	fs.BoolVar(
		&opts.DryRun,
		"dry-run",
		opts.DryRun,
		"Discover and validate power operations without changing resources",
	)
	fs.Func(
		"timeout",
		fmt.Sprintf(
			"Maximum time for the complete run (default %s)",
			configuration.DEFAULT_TIMEOUT,
		),
		func(value string) error {
			timeout, err := time.ParseDuration(value)
			if err == nil {
				opts.Timeout = &timeout
			}
			return err
		},
	)
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if fs.NArg() != 0 {
		return opts, fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	return opts, nil
}

// startProgress reports liveness even while discovery or SDK calls are waiting.
// Stop joins the goroutine before the diagnostic writer can be reused by callers.
func startProgress(ctx context.Context, log *slog.Logger, interval time.Duration) func() {
	done, stopped := make(chan struct{}), make(chan struct{})
	started := time.Now()
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				log.Debug("Run cancellation received", "error", ctx.Err())
				return
			case <-ticker.C:
				log.Debug("Run still active", "elapsed", time.Since(started))
			}
		}
	}()
	return func() { close(done); <-stopped }
}
