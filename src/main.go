package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/flynnkc/oci-frugal/src/pkg/authentication"
	"github.com/flynnkc/oci-frugal/src/pkg/controller"
	"github.com/flynnkc/oci-frugal/src/pkg/id"
	"github.com/flynnkc/oci-frugal/src/pkg/scheduler"
	"github.com/oracle/oci-go-sdk/v65/common"
)

const (
	ENVPREFIX         string = "FRUGAL_"
	ACTIONTYPE        string = "ACTION_TYPE"
	ALL               string = "ALL"
	ON                string = "ON"
	OFF               string = "OFF"
	LOGLEVEL          string = "LOG_LEVEL"
	REGION            string = "REGION"
	PRINCIPAL         string = "AUTH_TYPE"
	TAGNAMESPACE      string = "TAG_NAMESPACE"
	APIKEY            string = "api_key"
	INSTANCEPRINCIPAL string = "instance_principal"
	RESOURCEPRINCIPAL string = "resource_principal"
	WORKLOADPRINCIPAL string = "workload_principal"
)

var (
	// Type of authentication to use
	authType string
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

// Options is a collection of variables that affect behavior of the script
type Options struct {
	LogLevel     string // Logging level [debug, info, warn, error]
	Region       string // Region to run script on (Optional)
	Action       string // Select action(s) to take
	Principal    string // Principal type, Resource Principal if not set
	TagNamespace string // Tag Namespace to use, default Schedule
	log          *slog.Logger
}

func init() {
	usage := fmt.Sprintf("authentication type to use [%s, %s, %s, %s]",
		APIKEY, INSTANCEPRINCIPAL, RESOURCEPRINCIPAL, WORKLOADPRINCIPAL)
	flag.StringVar(&authType, "auth", "", usage)
	flag.StringVar(&region, "region", "", "region to run frugal on")
	flag.StringVar(&region, "r", "", "region to run frugal on (shorthand)")
}

func main() {
	flag.Parse()

	logLevel, ok := os.LookupEnv(fmt.Sprintf("%s%s", ENVPREFIX, LOGLEVEL))
	if !ok {
		logLevel = "INFO"
	}

	action, ok := os.LookupEnv(fmt.Sprintf("%s%s", ENVPREFIX, ACTIONTYPE))
	if !ok {
		action = ALL
	}

	tagNamespace, ok := os.LookupEnv(fmt.Sprintf("%s%s", ENVPREFIX, TAGNAMESPACE))
	if !ok {
		tagNamespace = "Schedule"
	}

	// Flags take priority over environment variables
	if authType == "" {
		if val, ok := os.LookupEnv(fmt.Sprintf("%s%s", ENVPREFIX, PRINCIPAL)); ok {
			authType = val
		} else {
			// default to resource principal for functions which lack command-line
			authType = RESOURCEPRINCIPAL
		}
	}

	if region == "" {
		if val, ok := os.LookupEnv(fmt.Sprintf("%s%s", ENVPREFIX, REGION)); ok {
			region = val
		}
	}

	opt := Options{
		LogLevel:     logLevel,
		Region:       region,
		Action:       action,
		Principal:    authType,
		TagNamespace: tagNamespace,
	}

	opt.log = setLogger(opt.LogLevel)
	slog.SetDefault(opt.log)
	opt.log.Info("Frugal started...")

	opt.log.Debug("Frugal initialized with arguments",
		"Log Level", opt.LogLevel,
		"Action", opt.Action,
		"Region", opt.Region,
		"Tag Namespace", opt.TagNamespace,
		"Principal", opt.Principal)

	run(&opt)
}

func run(opt *Options) {

	opt.log.Info("Supported Services", "Services", strings.Join(services, ", "))

	// Set region based on flag or get a list of subscribed regions
	var regions []string
	if opt.Region != "" {
		regions = append(regions, opt.Region)
		opt.log.Debug("Region specified in flags, not retrieving subscribed regions",
			"Region", regions[0])
	} else {

		var provider common.ConfigurationProvider
		if opt.Principal != "" {
			// Access to file based provider for debugging
			provider = common.DefaultConfigProvider()
		} else {
			// Resource principal provider for intended use case
			provider = authentication.NewDefaultProvider()
		}
		if provider == nil {
			opt.log.Error("default provider nil - exiting")
			os.Exit(1)
		}

		idClient, err := id.NewIdentityClient(provider)
		if err != nil {
			slog.Error("error getting identity client",
				"error", err)
		}

		regions, err := idClient.GetRegions()
		if err != nil {
			slog.Error("error getting regions",
				"error", err)
		}

		opt.log.Debug("Regions returned by client",
			"Regions", regions)
	}

	scheduler := scheduler.NewAnykeyNLScheduler()
	// Main control loop
	for i, r := range regions {
		opt.log.Info("BEGIN SCALING IN NEW REGION",
			"Region", r,
			"Order", i,
			"Region Count", len(regions))

		provider := authentication.NewRegionProvider(common.StringToRegion(r))
		controller, err := controller.NewTagController(provider, opt.TagNamespace)
		if err != nil {
			opt.log.Error("Unable to create controller",
				"error", err)
		}
		controller.SetScheduler(scheduler)
		controller.Run()
	}
}

// setLogger is just setting the logger type
func setLogger(level string) *slog.Logger {
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		log := slog.Default()
		log.Error("Invalid log level given - setting to warn")
		slogLevel = slog.LevelWarn
	}
	handler := slog.NewTextHandler(os.Stdout,
		&slog.HandlerOptions{Level: slogLevel})
	log := slog.New(handler)
	slog.SetDefault(log)
	return log
}
