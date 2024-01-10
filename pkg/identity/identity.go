package ident

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

type Identity struct {
	client   identity.IdentityClient
	tenantId string
}

// NewIdentityClient is an identity client generator
func NewIdentityClient(cfg common.ConfigurationProvider) (*Identity, error) {
	client, err := identity.NewIdentityClientWithConfigurationProvider(cfg)
	if err != nil {
		return nil, err
	}

	ocid, err := cfg.TenancyOCID()
	if err != nil {
		return nil, err
	}

	id := Identity{client: client, tenantId: ocid}
	return &id, nil
}

// GetRegions returns a slice containing subscribed region identifiers
// (ex. us-ashburn-1)
func (id *Identity) GetRegions() ([]string, error) {
	s := make([]string, 0)

	details := identity.ListRegionSubscriptionsRequest{TenancyId: common.String(id.tenantId)}
	response, err := id.client.ListRegionSubscriptions(context.Background(),
		details)
	if err != nil {
		return nil, err
	}

	regions := response.Items
	for _, region := range regions {
		s = append(s, *region.RegionName)
	}

	return s, nil
}
