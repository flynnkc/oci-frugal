package configuration

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/action"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
)

const (
	APIKEY            string = "api_key"
	INSTANCEPRINCIPAL string = "instance_principal"
	RESOURCEPRINCIPAL string = "resource_principal"
	WORKLOADPRINCIPAL string = "workload_principal"

	// Defaults
	DEFAULT_NAMESPACE string = "Schedule"
	DEFAULT_LOGLEVEL  string = "INFO"
	DEFAULT_SHUTDOWN  string = "soft"
	DEFAULT_TIMEOUT          = 10 * time.Minute

	// Scheduler
	NULL_SCHEDULER     string = "nullscheduler"
	ANYKEYNL_SCHEDULER string = "anykeynl"
)

type LogFunc func(...any) *slog.Logger

// Configuration is a collection of variables that affect behavior of the script.
// Configuration is responsible for validating and storing any configuration related
// variables. Default configurations should be set here.
type Configuration struct {
	timezone           *time.Location               // Timezone to run script against
	region             string                       // Region to run script on (Optional)
	tagNamespace       string                       // Tag Namespace to use, default Schedule
	schedule           string                       // Scheduler type
	action             action.Action                // Select action(s) to take
	principal          string                       // Principal type, Resource Principal if not set
	provider           common.ConfigurationProvider // Tag Namespace to use, default Schedule
	privateKeyPassword *string
	logFunc            func(...any) *slog.Logger
	logLevel           string
	dryRun             bool
	shutdown           string
	timeout            time.Duration
}

// ConfigurationOpts contains unvalidated inputs shared by CLI and API callers.
// Nil optional values receive defaults in NewConfiguration; explicit values are validated.
type ConfigurationOpts struct {
	LogLevel      *string        // Default Info
	Action        *string        // Default All
	TagNamespace  *string        // Default Schedule
	ConfigFile    *string        // Default ~/.oci/config
	ConfigProfile *string        // Default DEFAULT
	KeyPassword   *string        // Optional
	Principal     *string        // Default API Key
	Region        *string        // Optional
	Timezone      *string        // Default local timezone
	DryRun        bool           // Default false; plan without submitting power requests
	Shutdown      *string        // Default soft; Compute shutdown policy (soft or hard)
	Timeout       *time.Duration // Default 10 minutes for the complete run
}

func NewConfiguration(opts ConfigurationOpts) (*Configuration, error) {
	// Execution defaults and validation belong here so all callers use the same rules.
	shutdown := DEFAULT_SHUTDOWN
	if opts.Shutdown != nil {
		shutdown = *opts.Shutdown
	}
	if shutdown != "soft" && shutdown != "hard" {
		return nil, fmt.Errorf("shutdown must be soft or hard")
	}
	timeout := DEFAULT_TIMEOUT
	if opts.Timeout != nil {
		timeout = *opts.Timeout
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be positive")
	}

	// Log variables
	var logFunc LogFunc
	if opts.LogLevel == nil {
		opts.LogLevel = common.String("INFO")
		logFunc = StdTextLoggerInfo
	} else {
		switch strings.ToLower(*opts.LogLevel) {
		case "debug":
			logFunc = StdTextLoggerDebug
		case "info":
			logFunc = StdTextLoggerInfo
		case "warn":
			logFunc = StdTextLoggerWarn
		case "error":
			logFunc = StdTextLoggerError
		default:
			return nil, fmt.Errorf("invalid log level %q", *opts.LogLevel)
		}
	}

	var tz *time.Location
	if opts.Timezone == nil {
		tz = time.Local
	} else {
		t, err := time.LoadLocation(*opts.Timezone)
		if err != nil {
			return nil, err
		}
		tz = t
	}

	// Scheduler variables
	if opts.TagNamespace == nil {
		opts.TagNamespace = common.String(DEFAULT_NAMESPACE)
	}

	if strings.TrimSpace(*opts.TagNamespace) == "" {
		return nil, fmt.Errorf("tag namespace cannot be empty")
	}

	var act action.Action
	if opts.Action == nil {
		act = action.ALL
	} else {
		switch strings.ToLower(*opts.Action) {
		case "all":
			act = action.ALL
		case "on":
			act = action.ON
		case "off":
			act = action.OFF
		default:
			return nil, fmt.Errorf("invalid action %q: expected all, on, or off", *opts.Action)
		}
	}

	// Authentication variables
	if opts.ConfigFile == nil {
		opts.ConfigFile = common.String("~/.oci/config")
	}

	if opts.ConfigProfile == nil {
		opts.ConfigProfile = common.String("DEFAULT")
	}

	if opts.KeyPassword == nil {
		opts.KeyPassword = common.String("")
	}

	var provider common.ConfigurationProvider
	var err error
	if opts.Principal == nil {
		opts.Principal = common.String(APIKEY)
		provider, err = common.ConfigurationProviderFromFileWithProfile(
			*opts.ConfigFile,
			*opts.ConfigProfile,
			*opts.KeyPassword)
	} else {
		switch *opts.Principal {
		case APIKEY:
			provider, err = common.ConfigurationProviderFromFileWithProfile(*opts.ConfigFile,
				*opts.ConfigProfile,
				*opts.KeyPassword)
		case INSTANCEPRINCIPAL:
			provider, err = auth.InstancePrincipalConfigurationProvider()
		case RESOURCEPRINCIPAL:
			provider, err = auth.ResourcePrincipalConfigurationProvider()
		case WORKLOADPRINCIPAL:
			provider, err = auth.OkeWorkloadIdentityConfigurationProvider()
		default:
			err = fmt.Errorf("invalid principal type %s", *opts.Principal)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("error building configuration provider: %w", err)
	}

	// Define region
	// Blank will cause all regions to be searched
	if opts.Region == nil {
		opts.Region = common.String("")
	}

	o := Configuration{
		timezone:           tz,
		region:             *opts.Region,
		tagNamespace:       *opts.TagNamespace,
		schedule:           ANYKEYNL_SCHEDULER,
		action:             act,
		principal:          *opts.Principal,
		provider:           provider,
		privateKeyPassword: opts.KeyPassword,
		logFunc:            logFunc,
		logLevel:           *opts.LogLevel,
		dryRun:             opts.DryRun,
		shutdown:           shutdown,
		timeout:            timeout,
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
func (c *Configuration) Action() *action.Action {
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

// Timezone returns current time zone or default local timezone
func (c *Configuration) Timezone() *time.Location {
	return c.timezone
}

// MakeLog creates a logger with common attributes v[i], v[i+1]
func (c *Configuration) MakeLog(v ...any) *slog.Logger {
	return c.logFunc(v...)
}

func (c *Configuration) LogLevel() string {
	return c.logLevel
}

// DryRun reports whether execution should stop after read-only planning.
func (c *Configuration) DryRun() bool {
	return c.dryRun
}

// Shutdown returns the Compute shutdown policy: soft or hard.
func (c *Configuration) Shutdown() string {
	return c.shutdown
}

// Timeout bounds the complete run, independently of any HTTP request timeout.
func (c *Configuration) Timeout() time.Duration {
	return c.timeout
}
