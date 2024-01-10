package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/flynnkc/goci-frugal/pkg/authentication"
	configuration "github.com/flynnkc/goci-frugal/pkg/config"
	ident "github.com/flynnkc/goci-frugal/pkg/identity"
	"github.com/oracle/oci-go-sdk/v65/common"
	flag "github.com/spf13/pflag"
)

type ScalingType uint8

const (
	SCALE_ALL ScalingType = iota
	SCALE_UP
	SCALE_DOWN
)

var (
	authType string
	profile  string
	file     string
	logLevel string
	region   string
)

func main() {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	// Flags
	flag.StringVarP(&authType, "auth", "a", string(common.UserPrincipal),
		"Authentication type [user_principal, instance_principal]")
	flag.StringVarP(&profile, "profile", "p", "DEFAULT",
		"Profile to use from OCI config file")
	flag.StringVarP(&file, "file", "f",
		filepath.Join(usr.HomeDir, ".oci/config"),
		"OCI configuration file location")
	flag.StringVar(&logLevel, "log", "info",
		"Log level [debug, info, warn, error]")
	flag.StringVar(&region, "region", "", "Region Identifier to run script on")
	flag.Parse()

	log := setLogger(logLevel)
	slog.SetDefault(log)
	log.Info("Frugal started...")
	log.Debug("Frugal initialized with arguments",
		"Args", strings.Join(flag.Args(), ", "),
		"Auth Type", authType,
		"Profile", profile,
		"File", file,
		"Log Level", logLevel)

	if len(flag.Args()) < 1 {
		log.Error("No command given, use flag -h for usage instructions")
		os.Exit(1)
	}

	switch flag.Arg(0) {
	case "all":
		scaleCmd(SCALE_ALL)
	case "up":
		scaleCmd(SCALE_UP)
	case "down":
		scaleCmd(SCALE_DOWN)
	case "config":
		workConfig()
	default:
		log.Error("Unsupported command",
			"Cmd", flag.Arg(0))
	}
}

func scaleCmd(action ScalingType) {
	log := slog.Default()
	// Get a slice of services supported by script in search syntax
	services := getServices()
	log.Debug("Supported Services", "Services", strings.Join(services, ", "))

	cfg, err := authentication.NewConfigProvider(authType, profile, file)
	if err != nil {
		log.Error("Error encountered in new configuration provider",
			"Error", err)
	}

	idClient, err := ident.NewIdentityClient(cfg)
	if err != nil {
		slog.Error("Error getting identity client",
			"Error", err)
	}

	// Set region based on flag or get a list of subscribed regions
	regions := make([]string, 0)
	if region != "" {
		regions = append(regions, region)
		log.Debug("Region specified in flags, not retriving subscribed regions",
			"Region", regions[0])
	} else {
		regions, err := idClient.GetRegions()
		if err != nil {
			slog.Error("Error getting regions",
				"Error", err)
		}
		log.Debug("Regions returned by client",
			"Regions", regions)
	}

	// Main control loop
	for i, r := range regions {
		slog.Info("BEGIN SCALING IN NEW REGION",
			"Region", r,
			"Order", i,
			"Region Count", len(regions))
		// Controller goes here

	}
}

// workConfig is the function that works with configuration files
func workConfig() {
	log := slog.Default()
	data, err := configuration.LoadData(file)
	if err != nil {
		log.Error("Error loading configuration file",
			"File", file,
			"Error", err)
	}
	log.Debug("Configuration File Data", "Data", fmt.Sprintf("%+v", data))
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
		panic("Invalid log level given")
	}
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel})
	log := slog.New(handler)
	slog.SetDefault(log)
	return log
}

// getServices returns a slice containing services in search syntax.
// This is simply for convenience so new services can be added in an organized
// manner.
func getServices() []string {
	services := []string{
		"instance",
		"dbsystem",
		"autonomousdatabase",
		"analyticsinstance",
		"integrationinstance",
	}

	return services
}
