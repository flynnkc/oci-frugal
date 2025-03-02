// Package authentication handles OCI authentication logic
package authentication

import (
	"log/slog"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
)

var log *slog.Logger = slog.Default()

func NewDefaultProvider() auth.ConfigurationProviderWithClaimAccess {
	p, err := auth.ResourcePrincipalConfigurationProvider()
	if err != nil {
		log.Error("error retrieving Resource Principal provider",
			"error", err)
		return nil
	}

	return p
}

func NewRegionProvider(region common.Region) auth.ConfigurationProviderWithClaimAccess {
	p, err := auth.ResourcePrincipalConfigurationProviderForRegion(region)
	if err != nil {
		log.Error("error retriving Resource Principal provider",
			"error", err)
		return nil
	}

	return p
}
