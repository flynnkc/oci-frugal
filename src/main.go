package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/configuration"
	"github.com/flynnkc/oci-frugal/src/pkg/controller"
	"github.com/flynnkc/oci-frugal/src/pkg/id"
	"github.com/flynnkc/oci-frugal/src/pkg/scheduler"
)

const (
	ENVPREFIX    string = "FRUGAL_"
	ACTIONTYPE   string = "ACTION_TYPE"
	LOGLEVEL     string = "LOG_LEVEL"
	REGION       string = "REGION"
	PRINCIPAL    string = "AUTH_TYPE"
	TAGNAMESPACE string = "TAG_NAMESPACE"
	TIMEZONE     string = "TIMEZONE"
)

var (
	// Authentication variables
	authType      string
	configFile    string
	configProfile string

	region   string         // Region to run script against
	timezone *time.Location // Timezone script runs in/as

	services []string = []string{ // Supported services to be managed by the script
		"instance",
		"dbsystem",
		"autonomousdatabase",
		"analyticsinstance",
		"integrationinstance",
	}
)

func init() {
	usage := fmt.Sprintf("authentication type to use [%s, %s, %s, %s]",
		configuration.APIKEY,
		configuration.INSTANCEPRINCIPAL,
		configuration.RESOURCEPRINCIPAL,
		configuration.WORKLOADPRINCIPAL)
	flag.StringVar(&authType, "auth", "", usage)
	flag.StringVar(&region, "region", "", "region to run frugal on")
	flag.StringVar(&region, "r", "", "region to run frugal on (shorthand)")
	flag.StringVar(&configFile, "config", "", "OCI configuration file location")
	flag.StringVar(&configProfile, "profile", "", "OCI configuration file profile")
}

func main() {
	flag.Func("tz", "Timezone in a common format [ex. America/New York]",
		func(s string) error {
			tz, err := time.LoadLocation(s)
			if err != nil {
				return err
			}
			timezone = tz
			return nil
		})
	keyPass := flag.String("pass", "", "private key password for API Key Authentication")
	flag.Parse()

	logLevel := os.Getenv(fmt.Sprintf("%s%s", ENVPREFIX, LOGLEVEL))
	action := os.Getenv(fmt.Sprintf("%s%s", ENVPREFIX, ACTIONTYPE))
	tagNamespace := os.Getenv(fmt.Sprintf("%s%s", ENVPREFIX, TAGNAMESPACE))

	// Flags take priority over environment variables
	if authType == "" {
		if val, ok := os.LookupEnv(fmt.Sprintf("%s%s", ENVPREFIX, PRINCIPAL)); ok {
			authType = val
		}
	}

	if region == "" {
		if val, ok := os.LookupEnv(fmt.Sprintf("%s%s", ENVPREFIX, REGION)); ok {
			region = val
		}
	}

	cfg, err := configuration.NewConfiguration(
		logLevel,
		action,
		authType,
		configFile,
		configProfile,
		tagNamespace,
		keyPass)
	if err != nil {
		slog.Default().Error("error loading configuration: %w", "err", err)
		os.Exit(1)
	}

	log := cfg.MakeLog("Component", "main")
	if timezone == nil {
		if val, ok := os.LookupEnv(fmt.Sprintf("%s%s", ENVPREFIX, TIMEZONE)); ok {
			tz, err := time.LoadLocation(val)
			if err != nil {
				log.Error("error loading timezone from environment",
					"err", err)
				os.Exit(2)
			} else {
				cfg.SetTimezone(tz)
			}
		}
	}

	log.Info("Frugal started...")
	log.Debug("Frugal initialized with the following settings",
		"Log Level", logLevel,
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
	if region != "" {
		regions = append(regions, region)
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
