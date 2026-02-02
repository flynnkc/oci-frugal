package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/flynnkc/oci-frugal/src/pkg/configuration"
	"github.com/flynnkc/oci-frugal/src/pkg/controller"
	"github.com/flynnkc/oci-frugal/src/pkg/id"
	"github.com/flynnkc/oci-frugal/src/pkg/scheduler"
)

const (
	// Environment variables
	PREFIX       string = "FRUGAL_"
	ACTIONTYPE   string = "ACTION_TYPE"
	LOGLEVEL     string = "LOG_LEVEL"
	REGION       string = "REGION"
	PRINCIPAL    string = "AUTH_TYPE"
	FILE         string = "FILE"
	PROFILE      string = "PROFILE"
	KEYPASS      string = "KEY_PASS"
	TAGNAMESPACE string = "TAG_NAMESPACE"
	TIMEZONE     string = "TIMEZONE"
)

var (
	services []string = []string{ // Supported services to be managed by the script
		"instance",
		"dbsystem",
		"autonomousdatabase",
		"analyticsinstance",
		"integrationinstance",
	}
)

func main() {
	cfgOpts := setup()

	cfg, err := configuration.NewConfiguration(cfgOpts)
	if err != nil {
		slog.Default().Error("error loading configuration: %w", "err", err)
		os.Exit(1)
	}

	log := cfg.MakeLog("Component", "main")

	log.Info("Frugal started...")
	log.Debug("Frugal initialized with the following settings",
		"Log Level", *cfgOpts.LogLevel,
		"Region", cfg.Region(),
		"Tag Namespace", cfg.TagNamespace(),
		"Principal", cfg.AuthType(),
		"Scheduler", configuration.ANYKEYNL_SCHEDULER,
		"Action", cfg.Action(),
		"Timezone", cfg.Timezone())

	run(cfg)
}

func run(cfg *configuration.Configuration) {
	log := cfg.MakeLog("Component", "run")

	log.Info("Supported Services", "Services", strings.Join(services, ", "))

	// Set region based on flag/environment variable
	var regions []string
	if *cfg.Region() != "" {
		regions = append(regions, *cfg.Region())
		log.Debug("Region specified in flags, not retrieving subscribed regions",
			"Region", regions[0])
	} else {
		// Get list of subscribed regions
		idClient, err := id.NewIdentityClient(cfg.Provider())
		if err != nil {
			log.Error("error getting identity client",
				"error", err)
			os.Exit(1)
		}

		regions, err := idClient.GetRegions()
		if err != nil {
			log.Error("error getting regions",
				"error", err)
		}
		if len(regions) == 0 {
			log.Error("error no regions set")
			os.Exit(1)
		}

		log.Debug("Subscribed regions",
			"Regions", regions)
	}

	schFunc := scheduler.ScheduleFunc(*cfg.ScheduleType())
	sch := schFunc()

	// Main control loop
	lc := len(regions)
	var wg sync.WaitGroup
	for i, region := range regions {
		log.Info("BEGIN SCALING IN REGION",
			"Region", region,
			"Order", i,
			"Region Count", lc)

		controllerOpts := controller.ControllerOpts{
			ConfigurationProvider: cfg.Provider(),
			TagNamespace:          cfg.TagNamespace(),
			Scheduler:             sch,
			Log: cfg.MakeLog(
				"Component", "Controller",
				"Region", region),
		}

		controller, err := controller.NewTagController(controllerOpts)
		if err != nil {
			log.Error("Unable to create controller",
				"error", err)
		}
		controller.SetRegion(region)
		go controller.Run(&wg)
		wg.Add(1)
	}
	wg.Wait()
}

func setup() configuration.ConfigurationOpts {
	c := configuration.ConfigurationOpts{}
	// Add flag variables as first priority
	c = addFlags(c)
	// Add environment variables next
	c = addEnvironment(c)

	return c
}

func addFlags(c configuration.ConfigurationOpts) configuration.ConfigurationOpts {
	opts := c

	// TimeZone func
	flag.Func("tz", "Timezone in a common format [ex. America/New York]",
		func(s string) error {
			opts.Timezone = &s
			return nil
		})
	// Actions funcs
	flag.Func("action", "Action(s) to take on matching resources [all, on, off]",
		func(s string) error {
			opts.Action = &s
			return nil
		})

	// Authentication type
	authHelp := fmt.Sprintf("authentication type to use [%s, %s, %s, %s]",
		configuration.APIKEY,
		configuration.INSTANCEPRINCIPAL,
		configuration.RESOURCEPRINCIPAL,
		configuration.WORKLOADPRINCIPAL)
	flag.Func("auth", authHelp, func(s string) error {
		opts.Principal = &s
		return nil
	})

	// Region
	flag.Func("region", "region to run frugal on", func(s string) error {
		opts.Region = &s
		return nil
	})

	// Config File
	flag.Func("config-file", "OCI configuration file to use", func(s string) error {
		opts.ConfigFile = &s
		return nil
	})

	// Config Profile
	flag.Func("profiile", "OCI configuration profile to use", func(s string) error {
		opts.ConfigProfile = &s
		return nil
	})

	// Private Key Password
	flag.Func("key-pass", "password for private key linked to OCI config",
		func(s string) error {
			opts.KeyPassword = &s
			return nil
		})

	// Log Level
	flag.Func("v", "level to set logs", func(s string) error {
		opts.LogLevel = &s
		return nil
	})

	flag.Parse()

	return opts
}

func addEnvironment(c configuration.ConfigurationOpts) configuration.ConfigurationOpts {
	opts := c

	if opts.Timezone == nil {
		opts.Timezone = checkEnv(PREFIX + TIMEZONE)
	}

	if opts.Action == nil {
		opts.Action = checkEnv(PREFIX + ACTIONTYPE)
	}

	if opts.Principal == nil {
		opts.Principal = checkEnv(PREFIX + PRINCIPAL)
	}

	if opts.Region == nil {
		opts.Region = checkEnv(PREFIX + REGION)
	}

	if opts.ConfigFile == nil {
		opts.ConfigFile = checkEnv(PREFIX + FILE)
	}

	if opts.ConfigProfile == nil {
		opts.ConfigProfile = checkEnv(PREFIX + PROFILE)
	}

	if opts.KeyPassword == nil {
		opts.KeyPassword = checkEnv(PREFIX + KEYPASS)
	}

	if opts.LogLevel == nil {
		opts.LogLevel = checkEnv(PREFIX + LOGLEVEL)
	}

	return opts
}

func checkEnv(key string) *string {
	if v, ok := os.LookupEnv(key); ok {
		return &v
	}

	return nil
}
