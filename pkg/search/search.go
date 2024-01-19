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
	rsc := rs.ResourceSummaryCollection{Items: make([]rs.ResourceSummary, 0)}

	details := rs.StructuredSearchDetails{
		Query: common.String(query),
	}

	request := rs.SearchResourcesRequest{
		SearchDetails: details,
	}

	searchFunc := func(request rs.SearchResourcesRequest) (rs.SearchResourcesResponse,
		error) {
		return s.SearchResources(context.Background(), request)
	}

	// Pagination
	for r, err := searchFunc(request); ; r, err = searchFunc(request) {
		if err != nil {
			return &rsc, err
		}

		rsc.Items = append(rsc.Items, r.Items...)

		if r.OpcNextPage != nil {
			request.Page = r.OpcNextPage
		} else {
			break
		}
	}

	return &rsc, nil
}
