package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	flag "github.com/spf13/pflag"
)

func main() {
	authType := *flag.StringP("auth", "a", string(common.UserPrincipal),
		"Authentication type [Default: user_principal, instance_principal]")
	profile := *flag.StringP("profile", "p", "",
		"Profile to use from OCI config file")
	logLevel := *flag.String("log", "info",
		"Log level [debug, Default: info, warn, error]")
	flag.Parse()

	log := setLogger(logLevel)
	log.Info("Frugal started...")
	log.Debug("Frugal initialized with arguments",
		"Auth Type", authType,
		"Profile", profile,
		"Log Level", logLevel)
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
