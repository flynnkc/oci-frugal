package controller

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/controller/handlers"
	"github.com/flynnkc/oci-frugal/src/pkg/scheduler"
	"github.com/oracle/oci-go-sdk/v65/analytics"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/integration"
	rs "github.com/oracle/oci-go-sdk/v65/resourcesearch"
)

const (
	QUERY         string = "query instance, dbsystem, autonomousdatabase, analyticsinstance, integrationinstance resources"
	TC_WORK_QUEUE uint8  = 16
	TC_TIMEOUT           = 5 * time.Second
)

// TagController keeps track of all clients and scheduler interface for managing
// access, decisions, and actions on resources. Uses tags to manage schedules.
type TagController struct {
	tagNamespace string
	region       string
	scheduler    scheduler.Scheduler
	compute      core.ComputeClient
	database     database.DatabaseClient
	analytics    analytics.AnalyticsClient
	integration  integration.IntegrationInstanceClient
	search       rs.ResourceSearchClient
	log          *slog.Logger
}

// NewController initializes client snad returns a valid controller.
// If any clients fail to initialze, return nil controller and error.
func NewTagController(opts ControllerOpts) (*TagController, error) {
	// Verify required variables
	if opts.TagNamespace == nil || opts.ConfigurationProvider == nil ||
		opts.Region == nil {
		return nil, ErrControllerOptions
	}

	c := TagController{
		tagNamespace: *opts.TagNamespace,
		region:       *opts.Region,
	}

	// Prefer an expicit log but set default log if needed
	if opts.Log != nil {
		c.log = opts.Log
	} else {
		c.log = slog.Default()
	}

	// Create various resource clients
	// Compute
	instance, err := core.NewComputeClientWithConfigurationProvider(
		opts.ConfigurationProvider)
	if err != nil {
		return nil, err
	}
	c.compute = instance

	// Database
	db, err := database.NewDatabaseClientWithConfigurationProvider(
		opts.ConfigurationProvider)
	if err != nil {
		return nil, err
	}
	c.database = db

	// Analytics Cloud
	analytics, err := analytics.NewAnalyticsClientWithConfigurationProvider(
		opts.ConfigurationProvider)
	if err != nil {
		return nil, err
	}
	c.analytics = analytics

	// Integration Cloud
	i, err := integration.NewIntegrationInstanceClientWithConfigurationProvider(
		opts.ConfigurationProvider)
	if err != nil {
		return nil, err
	}
	c.integration = i

	// Resource Search
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

	// Return as many results in each result to minimize number of requests required
	request := rs.SearchResourcesRequest{
		SearchDetails: details,
		Limit:         common.Int(1000),
	}

	// Set context via wrapping SearchResources
	searchFunc := func(request rs.SearchResourcesRequest) (rs.SearchResourcesResponse,
		error) {
		ctx, cancel := context.WithTimeout(context.Background(), TC_TIMEOUT)
		defer cancel()

		return tc.search.SearchResources(ctx, request)
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
	tc.log.Debug("finished search")

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
func (tc *TagController) Run(controlWg *sync.WaitGroup) {
	defer controlWg.Done()
	tc.log.Info("Beginning TagController Run")

	// Search for supported resource types
	collection, err := tc.Search(QUERY)
	if err != nil {
		tc.log.Error("error searching for resources",
			slog.String("error", err.Error()))
		return
	}
	tc.log.Debug("items received from search",
		slog.Int("count", len(collection.Items)))

	// Make control objects, taskschannel for resources and WaitGroup to sync workers
	// with controller
	tasks := make(chan rs.ResourceSummary, TC_WORK_QUEUE)
	var workerWg sync.WaitGroup

	// Create workers
	for i := range TC_WORK_QUEUE {
		go tc.worker(i, tasks, &workerWg)
		workerWg.Add(1)
	}

	// Add items to tasks queue
	for _, item := range collection.Items {
		tasks <- item
	}

	// Send close signal to workers once out of items and wait for workers to finish
	close(tasks)
	workerWg.Wait()
}

func (tc *TagController) worker(id uint8, tasks <-chan rs.ResourceSummary,
	wg *sync.WaitGroup) {
	defer wg.Done()

	// Log attribute to identify worker
	logGroup := slog.Group("Worker",
		slog.Int("Worker ID", int(id)))
	tc.log.Info("Started Worker", logGroup)

	for {
		if task, more := <-tasks; more {
			tc.log.Info("Handling Resource", logGroup,
				slog.String("Resource ID", *task.Identifier),
				slog.String("Type", *task.ResourceType))

			switch *task.ResourceType {
			case "instance":
				handlers.HandleCompute(task)
			}
		} else {
			tc.log.Info("Work finished", logGroup)
			return
		}
	}
}
