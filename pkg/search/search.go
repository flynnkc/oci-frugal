package search

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/common"
	rs "github.com/oracle/oci-go-sdk/v65/resourcesearch"
)

type Search struct {
	rs.ResourceSearchClient
}

// NewSearch Client generates and returns a pointer to a resourcesearch client
func NewSearchClient(
	cfg common.ConfigurationProvider) (*Search, error) {
	client, err := rs.NewResourceSearchClientWithConfigurationProvider(cfg)
	if err != nil {
		return nil, err
	}

	return &Search{client}, nil
}

// ResourceSearch generates a search and returns a response
func (s *Search) ResourceSearch(query string) (*rs.ResourceSummaryCollection, error) {
	details := rs.StructuredSearchDetails{
		Query: common.String(query),
	}

	// TODO Pagination
	request := rs.SearchResourcesRequest{
		SearchDetails: details,
	}

	response, err := s.SearchResources(context.Background(), request)
	if err != nil {
		return nil, err
	}

	return &response.ResourceSummaryCollection, nil
}
