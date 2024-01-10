package main

import (
	"log/slog"
	"os"
	"strings"

	authentication "github.com/flynnkc/goci-frugal/pkg/auth"
	ident "github.com/flynnkc/goci-frugal/pkg/identity"
	"github.com/oracle/oci-go-sdk/v65/common"
	flag "github.com/spf13/pflag"
)

func main() {
	authType := *flag.StringP("auth", "a", string(common.UserPrincipal),
		"Authentication type [Default: user_principal, instance_principal]")
	profile := *flag.StringP("profile", "p", "",
		"Profile to use from OCI config file")
	file := *flag.StringP("file", "f", "",
		"Configuration file location [Default: ~/.oci/config]")
	logLevel := *flag.String("log", "info",
		"Log level [debug, Default: info, warn, error]")
	region := *flag.String("region", "", "Region Identifier to run script on")
	flag.Parse()

	log := setLogger(logLevel)
	slog.SetDefault(log)
	log.Info("Frugal started...")
	log.Debug("Frugal initialized with arguments",
		"Auth Type", authType,
		"Profile", profile,
		"Log Level", logLevel)

	cfg, err := authentication.NewConfigProvider(authType, profile, file)
	if err != nil {
		log.Error("Error encountered in new configuration provider",
			"Error", err)
	}

	if region != "" {
		doThing()
	}

	idClient, err := ident.NewIdentityClient(cfg)
	if err != nil {
		slog.Error("Error getting identity client",
			"Error", err)
	}

	regions, err := idClient.GetRegions()
	if err != nil {
		slog.Error("Error getting regions",
			"Error", err)
	}
	log.Debug("Regions returned by client",
		"Regions", regions)
}

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

func doThing() {
	log := slog.Default()
	log.Info("Doing thing")
}
