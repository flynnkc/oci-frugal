package id

import (
	"context"
	"errors"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type identityMock struct {
	identityAPI
	nsPages, tagPages, createdNS, createdTags int
	err                                       error
}

func (m *identityMock) ListRegionSubscriptions(
	ctx context.Context,
	_ identity.ListRegionSubscriptionsRequest,
) (identity.ListRegionSubscriptionsResponse, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("identity request has no deadline")
	}
	return identity.ListRegionSubscriptionsResponse{Items: []identity.RegionSubscription{
		{RegionName: common.String("ready"), Status: identity.RegionSubscriptionStatusReady},
		{
			RegionName: common.String("pending"),
			Status:     identity.RegionSubscriptionStatusInProgress,
		},
	}}, m.err
}
func (m *identityMock) ListTagNamespaces(
	_ context.Context,
	r identity.ListTagNamespacesRequest,
) (identity.ListTagNamespacesResponse, error) {
	m.nsPages++
	if r.Page == nil {
		return identity.ListTagNamespacesResponse{OpcNextPage: common.String("next")}, nil
	}
	return identity.ListTagNamespacesResponse{
		Items: []identity.TagNamespaceSummary{
			{
				Id:   common.String("ns"),
				Name: common.String("Schedule"),
			},
		},
	}, nil
}
func (m *identityMock) CreateTagNamespace(
	context.Context,
	identity.CreateTagNamespaceRequest,
) (identity.CreateTagNamespaceResponse, error) {
	m.createdNS++
	return identity.CreateTagNamespaceResponse{
		TagNamespace: identity.TagNamespace{
			Id: common.String("ns"),
		},
	}, nil
}
func (m *identityMock) ListTags(
	_ context.Context,
	r identity.ListTagsRequest,
) (identity.ListTagsResponse, error) {
	m.tagPages++
	if r.Page == nil {
		return identity.ListTagsResponse{OpcNextPage: common.String("next")}, nil
	}
	return identity.ListTagsResponse{
		Items: []identity.TagSummary{
			{
				Name: common.String("AnyDay"),
			},
		},
	}, nil
}
func (m *identityMock) CreateTag(
	_ context.Context,
	r identity.CreateTagRequest,
) (identity.CreateTagResponse, error) {
	if *r.TagNamespaceId != "ns" || *r.Name != "Monday" {
		panic("wrong tag definition")
	}
	m.createdTags++
	return identity.CreateTagResponse{}, m.err
}
func TestReadyRegionsAndErrors(t *testing.T) {
	m := &identityMock{}
	id := &Identity{client: m, tenantId: "tenant"}
	regions, err := id.GetRegions(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"ready"}, regions)
	m.err = errors.New("denied")
	_, err = id.GetRegions(context.Background())
	assert.ErrorIs(t, err, m.err)
}
func TestEnsureActualTagDefinitions(t *testing.T) {
	m := &identityMock{}
	id := &Identity{client: m, tenantId: "tenant"}
	// AnyDay already exists on a later page; duplicate requested keys are created once.
	err := id.CreateOrUpdateTagNamespace(context.Background(), "Schedule", "", []string{
		"AnyDay",
		"Monday",
		"Monday",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, m.nsPages)
	assert.Equal(t, 2, m.tagPages)
	assert.Zero(t, m.createdNS)
	assert.Equal(t, 1, m.createdTags)
	m.err = errors.New("create denied")
	err = id.CreateOrUpdateTagNamespace(context.Background(), "Schedule", "ns", []string{
		"Monday",
	})
	assert.ErrorIs(t, err, m.err)
}
