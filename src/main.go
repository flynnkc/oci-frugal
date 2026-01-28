package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
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
	// Region to run script against
	region string
	// Supported services to be managed by the script
	services []string = []string{
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
		os.Exit(1)
	}

	slog.SetDefault(cfg.Log)
	cfg.Log.Info("Frugal started...")
	cfg.Log.Debug("Frugal initialized with arguments",
		"Log Level", cfg.LogLevel,
		"Action", cfg.Action(),
		"Region", cfg.Region(),
		"Tag Namespace", cfg.TagNamespace(),
		"Principal", cfg.AuthType())

	run(cfg)
}

func run(cfg *configuration.Configuration) {

	cfg.Log.Info("Supported Services", "Services", strings.Join(services, ", "))

	// Set region based on flag/environment variable
	var regions []string
	if region != "" {
		regions = append(regions, region)
		cfg.Log.Debug("Region specified in flags, not retrieving subscribed regions",
			"Region", regions[0])
	} else {
		// Get list of subscribed regions
		idClient, err := id.NewIdentityClient(cfg.Provider())
		if err != nil {
			cfg.Log.Error("error getting identity client",
				"error", err)
			os.Exit(1)
		}

		regions, err := idClient.GetRegions()
		if err != nil {
			cfg.Log.Error("error getting regions",
				"error", err)
		}
		if len(regions) == 0 {
			os.Exit(1)
		}

		cfg.Log.Debug("Regions returned by client",
			"Regions", regions)
	}

	// Build scheduler with configurable timezone (single TZ for entire run)
	var sched scheduler.AnykeyNLScheduler
	if tz := os.Getenv(fmt.Sprintf("%s%s", ENVPREFIX, TIMEZONE)); tz != "" {
		if loc, err := time.LoadLocation(tz); err != nil {
			cfg.Log.Error("Invalid timezone provided; falling back to local",
				"timezone", tz,
				"error", err)
			sched = scheduler.NewAnykeyNLScheduler()
		} else {
			sched = scheduler.NewAnykeyNLSchedulerWithLocation(loc)
		}
	} else {
		sched = scheduler.NewAnykeyNLScheduler()
	}
	// Main control loop
	for i, r := range regions {
		cfg.Log.Info("BEGIN SCALING IN NEW REGION",
			"Region", r,
			"Order", i,
			"Region Count", len(regions))

		provider := cfg.Provider()
		controller, err := controller.NewTagController(provider, cfg.TagNamespace())
		if err != nil {
			cfg.Log.Error("Unable to create controller",
				"error", err)
		}
		controller.SetScheduler(sched)
		controller.Run()
	}
}
