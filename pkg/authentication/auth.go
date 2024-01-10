// Package authentication handles OCI authentication logic
package authentication

import (
	"errors"
	"log/slog"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
)

const DEFAULT_CONFIG string = "DEFAULT"

func NewConfigProvider(authType, profile, file string) (common.ConfigurationProvider, error) {
	log := slog.Default()
	log.Debug("Creating new Configuration Provider",
		"Auth Type", authType,
		"Profile", profile,
		"Config File", file)

	switch authType {
	case string(common.UserPrincipal):
		return common.ConfigurationProviderFromFileWithProfile(
			file, profile, "")
	case string(common.InstancePrincipal):
		return auth.InstancePrincipalConfigurationProvider()
	default:
		return nil, errors.New("invalid authentication type provided")
	}
}
