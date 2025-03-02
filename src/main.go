package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/flynnkc/oci-frugal/src/pkg/authentication"
	"github.com/flynnkc/oci-frugal/src/pkg/id"
)

// Options is a collection of variables that affect behavior of the script
type Options struct {
	logLevel string // Logging level [debug, info, warn, error]
	region   string // Region to run script on (Optional)
	action   string
	log      *slog.Logger
}

const (
	ACTIONTYPE string = "ACTION_TYPE"
	ALL        string = "ALL"
	ON         string = "ON"
	OFF        string = "OFF"
	LOGLEVEL   string = "LOG_LEVEL"
	REGION     string = "REGION"
)

// Supported services to be managed by the script
var services []string = []string{
	"instance",
	"dbsystem",
	"autonomousdatabase",
	"analyticsinstance",
	"integrationinstance",
}

func main() {
	logLevel, ok := os.LookupEnv(LOGLEVEL)
	if !ok {
		logLevel = "INFO"
	}

	action, ok := os.LookupEnv(ACTIONTYPE)
	if !ok {
		action = ALL
	}

	opt := Options{
		logLevel: logLevel,
		region:   os.Getenv(REGION),
		action:   action,
	}

	opt.log = setLogger(opt.logLevel)
	slog.SetDefault(opt.log)
	opt.log.Info("Frugal started...")

	opt.log.Debug("Frugal initialized with arguments",
		"Log Level", opt.logLevel,
		"Action", opt.action)

	run(&opt)
}

func run(opt *Options) {

	opt.log.Debug("Supported Services", "Services", strings.Join(services, ", "))

	// Set region based on flag or get a list of subscribed regions
	var regions []string
	if opt.region != "" {
		regions = append(regions, opt.region)
		opt.log.Debug("Region specified in flags, not retrieving subscribed regions",
			"Region", regions[0])
	} else {
		provider := authentication.NewDefaultProvider()
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

	// Main control loop
	for i, r := range regions {
		opt.log.Info("BEGIN SCALING IN NEW REGION",
			"Region", r,
			"Order", i,
			"Region Count", len(regions))
		// Controller goes here

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
