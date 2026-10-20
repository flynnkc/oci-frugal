package id

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

const requestTimeout = 30 * time.Second

type identityAPI interface {
	ListRegionSubscriptions(
		context.Context,
		identity.ListRegionSubscriptionsRequest,
	) (identity.ListRegionSubscriptionsResponse, error)
	ListTagNamespaces(
		context.Context,
		identity.ListTagNamespacesRequest,
	) (identity.ListTagNamespacesResponse, error)
	CreateTagNamespace(
		context.Context,
		identity.CreateTagNamespaceRequest,
	) (identity.CreateTagNamespaceResponse, error)
	ListTags(context.Context, identity.ListTagsRequest) (identity.ListTagsResponse, error)
	CreateTag(context.Context, identity.CreateTagRequest) (identity.CreateTagResponse, error)
}
type Identity struct {
	client   identityAPI
	tenantId string
}

func NewIdentityClient(cfg common.ConfigurationProvider) (*Identity, error) {
	if cfg == nil {
		return nil, fmt.Errorf("identity configuration provider is required")
	}
	client, err := identity.NewIdentityClientWithConfigurationProvider(cfg)
	if err != nil {
		return nil, err
	}
	tenant, err := cfg.TenancyOCID()
	if err != nil {
		return nil, err
	}
	return &Identity{client: client, tenantId: tenant}, nil
}

func (id *Identity) GetRegions(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	resp, err := id.client.ListRegionSubscriptions(ctx, identity.ListRegionSubscriptionsRequest{
		TenancyId: &id.tenantId,
	})
	if err != nil {
		return nil, err
	}
	regions := make([]string, 0, len(resp.Items))
	seen := make(map[string]bool)
	for _, r := range resp.Items {
		if r.Status != identity.RegionSubscriptionStatusReady {
			continue
		}
		if r.RegionName == nil || *r.RegionName == "" {
			return nil, fmt.Errorf("ready region has no name")
		}
		if !seen[*r.RegionName] {
			regions = append(regions, *r.RegionName)
			seen[*r.RegionName] = true
		}
	}
	return regions, nil
}

// CreateOrUpdateTagNamespace ensures that the namespace and tag key definitions
// exist. DefinedTags on a namespace describe that namespace; they do not define
// its keys. This helper is opt-in and is never called by scheduling or dry runs.
func (id *Identity) CreateOrUpdateTagNamespace(
	ctx context.Context,
	name, nsid string,
	keys []string,
) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("namespace name is required")
	}
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("tag key cannot be empty")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if nsid == "" {
		req := identity.ListTagNamespacesRequest{CompartmentId: &id.tenantId}
		seen := make(map[string]bool)
		for {
			resp, err := id.client.ListTagNamespaces(ctx, req)
			if err != nil {
				return err
			}
			for _, ns := range resp.Items {
				if ns.Name != nil && *ns.Name == name {
					if ns.IsRetired != nil && *ns.IsRetired {
						return fmt.Errorf("namespace %q is retired", name)
					}
					if ns.Id == nil || *ns.Id == "" {
						return fmt.Errorf("namespace %q is missing ID", name)
					}
					nsid = *ns.Id
					break
				}
			}
			if nsid != "" || resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
				break
			}
			if seen[*resp.OpcNextPage] {
				return fmt.Errorf("repeated namespace pagination token")
			}
			seen[*resp.OpcNextPage] = true
			req.Page = resp.OpcNextPage
		}
		if nsid == "" {
			resp, err := id.client.CreateTagNamespace(ctx, identity.CreateTagNamespaceRequest{
				CreateTagNamespaceDetails: identity.CreateTagNamespaceDetails{
					CompartmentId: &id.tenantId,
					Name:          &name,
					Description:   common.String("Created by oci-frugal"),
				},
			})
			if err != nil {
				return err
			}
			if resp.Id == nil || *resp.Id == "" {
				return fmt.Errorf("created namespace is missing ID")
			}
			nsid = *resp.Id
		}
	}
	existing := make(map[string]bool)
	req := identity.ListTagsRequest{TagNamespaceId: &nsid}
	seen := make(map[string]bool)
	for {
		resp, err := id.client.ListTags(ctx, req)
		if err != nil {
			return err
		}
		for _, tag := range resp.Items {
			if tag.Name != nil {
				if tag.IsRetired != nil && *tag.IsRetired {
					for _, key := range keys {
						if key == *tag.Name {
							return fmt.Errorf("tag key %q is retired", key)
						}
					}
				} else {
					existing[*tag.Name] = true
				}
			}
		}
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		if seen[*resp.OpcNextPage] {
			return fmt.Errorf("repeated tag pagination token")
		}
		seen[*resp.OpcNextPage] = true
		req.Page = resp.OpcNextPage
	}
	for _, key := range keys {
		if existing[key] {
			continue
		}
		_, err := id.client.CreateTag(ctx, identity.CreateTagRequest{
			TagNamespaceId: &nsid,
			CreateTagDetails: identity.CreateTagDetails{
				Name:        common.String(key),
				Description: common.String("Power schedule managed by oci-frugal"),
			},
		})
		if err != nil {
			return fmt.Errorf("create tag %s: %w", key, err)
		}
		existing[key] = true
	}
	return nil
}
