package controller

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"github.com/flynnkc/oci-frugal/src/pkg/controller/handlers"
	"github.com/flynnkc/oci-frugal/src/pkg/controller/task"
	"github.com/flynnkc/oci-frugal/src/pkg/scheduler"
	"github.com/oracle/oci-go-sdk/v65/analytics"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/integration"
	rs "github.com/oracle/oci-go-sdk/v65/resourcesearch"
)

const (
	query string = "query instance, dbsystem, autonomousdatabase, analyticsinstance, integrationinstance resources"
)

// TagController keeps track of all clients and scheduler interface for managing
// access, decisions, and actions on resources. Uses tags to manage schedules.
type TagController struct {
	scheduler    scheduler.Scheduler
	compute      core.ComputeClient
	database     database.DatabaseClient
	analytics    analytics.AnalyticsClient
	integration  integration.IntegrationInstanceClient
	search       rs.ResourceSearchClient
	tagNamespace string
	log          *slog.Logger
}

// NewController initializes client snad returns a valid controller.
// If any clients fail to initialze, return nil controller and error.
func NewTagController(opts ControllerOpts) (*TagController, error) {
	// Verify required variables
	if opts.TagNamespace == nil || opts.ConfigurationProvider == nil {
		return nil, ErrControllerOptions
	}

	c := TagController{
		tagNamespace: *opts.TagNamespace,
	}

	// Prefer an expicit log
	if opts.Log != nil {
		c.log = opts.Log
	} else {
		c.log = slog.Default()
	}
	instance, err := core.NewComputeClientWithConfigurationProvider(
		opts.ConfigurationProvider)
	if err != nil {
		return nil, err
	}
	c.compute = instance

	db, err := database.NewDatabaseClientWithConfigurationProvider(
		opts.ConfigurationProvider)
	if err != nil {
		return nil, err
	}
	c.database = db

	analytics, err := analytics.NewAnalyticsClientWithConfigurationProvider(
		opts.ConfigurationProvider)
	if err != nil {
		return nil, err
	}
	c.analytics = analytics

	i, err := integration.NewIntegrationInstanceClientWithConfigurationProvider(
		opts.ConfigurationProvider)
	if err != nil {
		return nil, err
	}
	c.integration = i

	s, err := rs.NewResourceSearchClientWithConfigurationProvider(
		opts.ConfigurationProvider)
	if err != nil {
		return nil, err
	}
	c.search = s

	// Default to null scheduler as last resort
	if opts.Scheduler != nil {
		c.scheduler = opts.Scheduler
	} else {
		c.scheduler = &scheduler.NullScheduler{}
	}

	return &c, nil
}

// SetScheduler sets the scheduler to be used for parsing run schedules
func (tc *TagController) SetScheduler(sch scheduler.Scheduler) *TagController {
	tc.log.Debug("setting new scheduler",
		"type", sch.Type())

	tc.scheduler = sch
	return tc
}

// Search generates a structured search and returns a resource summary collection
func (tc *TagController) Search(query string) (rs.ResourceSummaryCollection, error) {
	rsc := rs.ResourceSummaryCollection{Items: make([]rs.ResourceSummary, 0)}

	tc.log.Debug("searching for resources",
		"query", query)
	details := rs.StructuredSearchDetails{
		Query: common.String(query),
	}

	request := rs.SearchResourcesRequest{
		SearchDetails: details,
		Limit:         common.Int(1000),
	}

	searchFunc := func(request rs.SearchResourcesRequest) (rs.SearchResourcesResponse,
		error) {
		return tc.search.SearchResources(context.Background(), request)
	}

	// Pagination by breaking when no next page
	tc.log.Debug("preparing to send search requests")
	for r, err := searchFunc(request); ; r, err = searchFunc(request) {
		tc.log.Debug("search response",
			slog.Int("status", r.RawResponse.StatusCode),
			slog.String("next page", *r.OpcNextPage))
		if err != nil {
			return rsc, err
		}

		rsc.Items = append(rsc.Items, r.Items...)

		if r.OpcNextPage != nil {
			request.Page = r.OpcNextPage
		} else {
			break
		}
	}
	tc.log.Debug("finished search",
		slog.Int("num results", len(rsc.Items)))

	return rsc, nil
}

func (tc *TagController) SetRegion(region string) {
	tc.analytics.SetRegion(region)
	tc.compute.SetRegion(region)
	tc.database.SetRegion(region)
	tc.integration.SetRegion(region)
	tc.search.SetRegion(region)
}

// Run starts the controller spawning workers and queuing tasks
func (tc *TagController) Run(wait *sync.WaitGroup) {
	tc.log.Info("Beginning TagController Run")
	var wg sync.WaitGroup

	// Search query where clause
	where := "where definedTags.Namespace = '%s'"

	// Hold resources found by search
	items := make([]rs.ResourceSummary, 0)

	// Channels for tasks and results
	tasks := make(chan task.Task, numWorkers)

	rsc, err := tc.Search(fmt.Sprintf(query+where, tc.tagNamespace))
	tc.log.Error("error in search",
		"error", err,
		"items returned", strconv.Itoa(len(rsc.Items)))
	items = append(items, rsc.Items...)

	// Start workers
	for range numWorkers {
		wg.Add(1)
		go func(tasks <-chan task.Task) {
			defer wg.Done()
			for {
				t, more := <-tasks
				if !more {
					return
				}

				switch *t.Resource.ResourceType {
				case "instance":
					handlers.HandleCompute(t)
				default:
					tc.log.Warn("Unknown type detected",
						"type", *t.Resource.ResourceType)
				}
			}
		}(tasks)
	}

	// Send tasks
	for _, t := range items {
		// Evaluate results
		action, err := tc.scheduler.Evaluate(
			t.DefinedTags[tc.tagNamespace])
		if err != nil {
			slog.Error("error evaluating schedule",
				"error", err)
			continue
		} else if action == scheduler.NULL_ACTION {
			continue
		}

		tasks <- task.Task{Action: action, Resource: t}
	}

	tc.log.Info("Finished compute")
	close(tasks)

	wg.Wait()
}
