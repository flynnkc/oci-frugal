package id

import (
	"context"
	"errors"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

type Identity struct {
	identity.IdentityClient
	tenantId string
}

var ErrNo2xxStatus = errors.New("unsuccessful response status code")

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

	id := Identity{client, ocid}
	return &id, nil
}

// GetRegions returns a slice containing subscribed region identifiers
// (ex. us-ashburn-1)
func (id *Identity) GetRegions() ([]string, error) {
	s := make([]string, 0)

	details := identity.ListRegionSubscriptionsRequest{TenancyId: common.String(id.tenantId)}
	response, err := id.ListRegionSubscriptions(context.Background(),
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

// CreateOrUpdateTagNamespace updates a tag namespace or creates it if the
// namespace does not exist; Takes ns name, keys, and namespace OCID nsid
// if known
func (id *Identity) CreateOrUpdateTagNamespace(ns, nsid string,
	keys map[string]map[string]any) error {
	if nsid == "" {
		// List Tag Namespaces and pick out correct one
		request := identity.ListTagNamespacesRequest{
			CompartmentId: &id.tenantId,
		}

		response, err := id.ListTagNamespaces(context.Background(), request)
		if err != nil {
			return err
		}

		for _, item := range response.Items {
			if *item.Name == ns {
				nsid = *item.Id
				break
			}
		}

		// Create namespace
		if nsid == "" {
			response, err := id.CreateNs(ns, keys)
			if err != nil {
				return err
			} else if response.RawResponse.StatusCode < 200 &&
				response.RawResponse.StatusCode > 299 {
				return ErrNo2xxStatus
			} else {
				return nil
			}
		}
	}

	// Update namespace
	response, err := id.UpdateNs(&nsid, keys)
	if err != nil {
		return err
	} else if response.RawResponse.StatusCode < 200 &&
		response.RawResponse.StatusCode > 299 {
		return ErrNo2xxStatus
	}

	return nil
}

// CreateNs creates a new namespace from a string and map
func (id *Identity) CreateNs(name string, tags map[string]map[string]any) (
	identity.CreateTagNamespaceResponse, error) {
	details := identity.CreateTagNamespaceDetails{
		CompartmentId: common.String(id.tenantId),
		Name:          common.String(name),
		Description:   common.String("Created by oci-frugal"),
		DefinedTags:   tags,
	}

	request := identity.CreateTagNamespaceRequest{
		CreateTagNamespaceDetails: details}

	response, err := id.CreateTagNamespace(context.Background(), request)
	if err != nil {
		return identity.CreateTagNamespaceResponse{}, err
	}

	return response, nil
}

// UpdateNs updates a namespace to include new keys returning response and error
func (id *Identity) UpdateNs(nsid *string, tags map[string]map[string]any) (
	identity.UpdateTagNamespaceResponse, error) {
	details := identity.UpdateTagNamespaceDetails{DefinedTags: tags}

	request := identity.UpdateTagNamespaceRequest{
		TagNamespaceId:            nsid,
		UpdateTagNamespaceDetails: details,
	}

	response, err := id.UpdateTagNamespace(context.Background(), request)
	if err != nil {
		return identity.UpdateTagNamespaceResponse{}, err
	}

	return response, nil
}
