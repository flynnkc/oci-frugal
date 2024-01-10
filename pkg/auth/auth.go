// Package authentication handles OCI authentication logic
package authentication

import (
	"errors"
	"log/slog"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
)

func NewConfigProvider(authType, profile, file string) (common.ConfigurationProvider, error) {
	log := slog.Default()
	log.Debug("Creating new Configuration Provider",
		"Auth Type", authType,
		"Profile", profile,
		"Config File", file)
	switch authType {
	case string(common.UserPrincipal):
		//Most to least complicated
		if profile != "" && file != "" {
			// Custom File and Profile
			return common.ConfigurationProviderFromFileWithProfile(
				file, profile, "")
		} else if profile != "" {
			// Custom Profile
			// TODO Filepath handling
			return common.ConfigurationProviderFromFileWithProfile(
				"!/.oci/config", profile, "")
		} else if file != "" {
			// Custom config file
			return common.ConfigurationProviderFromFileWithProfile(
				file, "DEFAULT", "")
		} else {
			common.DefaultConfigProvider()
		}
	case string(common.InstancePrincipal):
		return auth.InstancePrincipalConfigurationProvider()
	default:
		return nil, errors.New("invalid authentication type provided")
	}

	return nil, errors.New("invalid authentication type provided")
}
