package configuration

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
)

const (
	ALL               string = "ALL"
	ON                string = "ON"
	OFF               string = "OFF"
	APIKEY            string = "api_key"
	INSTANCEPRINCIPAL string = "instance_principal"
	RESOURCEPRINCIPAL string = "resource_principal"
	WORKLOADPRINCIPAL string = "workload_principal"

	// Defaults
	DEFAULT_NAMESPACE string = "Schedule"
	DEFAULT_LOGLEVEL  string = "INFO"

	// Scheduler
	NULL_SCHEDULER     string = "nullscheduler"
	ANYKEYNL_SCHEDULER string = "anykeynl"
)

// Configuration is a collection of variables that affect behavior of the script.
// Configuration is responsible for validating and storing any configuration related
// variables. Default configurations should be set here.
type Configuration struct {
	timezone           *time.Location               // Timezone to run script against
	region             string                       // Region to run script on (Optional)
	tagNamespace       string                       // Tag Namespace to use, default Schedule
	schedule           string                       // Scheduler type
	action             string                       // Select action(s) to take
	principal          string                       // Principal type, Resource Principal if not set
	provider           common.ConfigurationProvider // Tag Namespace to use, default Schedule
	privateKeyPassword *string
	logFunc            func(...any) *slog.Logger
}

func NewConfiguration(logLevel string,
	action string,
	principal string,
	file string,
	profile string,
	tagNamespace string,
	keyPass *string) (*Configuration, error) {
	// Log variables
	var logFunc func(...any) *slog.Logger
	switch logLevel {
	case "debug":
		logFunc = StdTextLoggerDebug
	case "info":
		logFunc = StdTextLoggerInfo
	case "warn":
		logFunc = StdTextLoggerWarn
	case "error":
		logFunc = StdTextLoggerError
	default:
		logFunc = StdTextLoggerInfo
	}

	// Scheduler variables
	if action == "" {
		action = ALL
	}

	if tagNamespace == "" {
		tagNamespace = DEFAULT_NAMESPACE
	}

	// Authentication variables
	if file == "" {
		file = "~/.oci/config"
	}

	if profile == "" {
		profile = "DEFAULT"
	}

	var provider common.ConfigurationProvider
	var err error
	switch principal {
	case APIKEY:
		provider, err = common.ConfigurationProviderFromFileWithProfile(file, profile, *keyPass)
	case INSTANCEPRINCIPAL:
		provider, err = auth.InstancePrincipalConfigurationProvider()
	case RESOURCEPRINCIPAL:
		provider, err = auth.ResourcePrincipalConfigurationProvider()
	case WORKLOADPRINCIPAL:
		provider, err = auth.OkeWorkloadIdentityConfigurationProvider()
	default:
		provider, err = common.ConfigurationProviderFromFileWithProfile(file, profile, *keyPass)
	}
	if err != nil {
		return nil, fmt.Errorf("error building configuration provider: %w", err)
	}

	region, err := provider.Region()
	if err != nil {
		return nil, fmt.Errorf("error getting region from provider: %w", err)
	}

	o := Configuration{
		timezone:           time.Local,
		region:             region,
		tagNamespace:       tagNamespace,
		schedule:           ANYKEYNL_SCHEDULER,
		action:             action,
		principal:          principal,
		provider:           provider,
		privateKeyPassword: keyPass,
		logFunc:            logFunc,
	}

	return &o, nil
}

// Region returns the default configured region
func (c *Configuration) Region() *string {
	return &c.region
}

// TagNamespace returns the configured tag namespace
func (c *Configuration) TagNamespace() *string {
	return &c.tagNamespace
}

// Action returns the configured action [UP, DOWN, ALL]
func (c *Configuration) Action() *string {
	return &c.action
}

// AuthType returns the configured authentication type
func (c *Configuration) AuthType() *string {
	return &c.principal
}

// Provider returns the default configured provider
func (c *Configuration) Provider() common.ConfigurationProvider {
	return c.provider
}

// ScheduleType returns of the type of scheduler in use
func (c *Configuration) ScheduleType() *string {
	return &c.schedule
}

// SetTimezone sets the configured time zone
func (c *Configuration) SetTimezone(tz *time.Location) {
	c.timezone = tz
}

// Timezone returns current time zone or default local timezone
func (c *Configuration) Timezone() *time.Location {
	return c.timezone
}

// MakeLog creates a logger with common attributes v[i], v[i+1]
func (c *Configuration) MakeLog(v ...any) *slog.Logger {
	return c.logFunc(v...)
}
