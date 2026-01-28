package configuration

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"crypto/x509"
	"encoding/pem"

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
)

// Options is a collection of variables that affect behavior of the script
type Configuration struct {
	Log                *slog.Logger
	LogLevel           string                       // Logging level [debug, info, warn, error]
	region             string                       // Region to run script on (Optional)
	tagNamespace       string                       // Tag Namespace to use, default Schedule
	action             string                       // Select action(s) to take
	principal          string                       // Principal type, Resource Principal if not set
	provider           common.ConfigurationProvider // Tag Namespace to use, default Schedule
	privateKeyPassword *string
}

func NewConfiguration(logLevel string,
	action string,
	principal string,
	file string,
	profile string,
	tagNamespace string,
	keyPass *string) (*Configuration, error) {
	if logLevel == "" {
		logLevel = DEFAULT_LOGLEVEL
	}

	if action == "" {
		action = ALL
	}

	if tagNamespace == "" {
		tagNamespace = DEFAULT_NAMESPACE
	}

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
		return nil, fmt.Errorf("error unsupported auth type %s", principal)
	}
	if err != nil {
		return nil, fmt.Errorf("error building configuration provider: %w", err)
	}

	region, err := provider.Region()
	if err != nil {
		return nil, fmt.Errorf("error getting region from provider: %w", err)
	}

	o := Configuration{
		Log:                setLogger(logLevel),
		region:             region,
		tagNamespace:       tagNamespace,
		action:             action,
		principal:          principal,
		provider:           provider,
		privateKeyPassword: keyPass,
	}

	return &o, nil
}

// Region returns the default configured region
func (c *Configuration) Region() string {
	return c.region
}

// TagNamespace returns the configured tag namespace
func (c *Configuration) TagNamespace() string {
	return c.tagNamespace
}

// Action returns the configured action [UP, DOWN, ALL]
func (c *Configuration) Action() string {
	return c.action
}

// AuthType returns the configured authentication type
func (c *Configuration) AuthType() string {
	return c.principal
}

// Provider returns the default configured provider
func (c *Configuration) Provider() common.ConfigurationProvider {
	return c.provider
}

// ForRegion returns a modified configuration provider for the selected region
func (c *Configuration) ForRegion(region string) (common.ConfigurationProvider, error) {
	// Only API key auth needs a new provider with a different region.
	if c.principal == APIKEY {
		tenant, err := c.provider.TenancyOCID()
		if err != nil {
			return nil, err
		}

		user, err := c.provider.UserOCID()
		if err != nil {
			return nil, err
		}

		fp, err := c.provider.KeyFingerprint()
		if err != nil {
			return nil, err
		}

		// Get the RSA key and convert to PEM string expected by NewRawConfigurationProvider
		pk, err := c.provider.PrivateRSAKey()
		if err != nil {
			return nil, err
		}
		pemBytes := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(pk),
		})

		return common.NewRawConfigurationProvider(
			tenant,
			user,
			region,
			fp,
			string(pemBytes),
			c.privateKeyPassword,
		), nil
	}

	// For non-API key principals (instance/resource/workload), return the existing provider.
	// Region is derived by the underlying environment and typically cannot be overridden here.
	return c.provider, nil
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
