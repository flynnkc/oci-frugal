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
	computeQuery      string = "query instance resources"
	dbsystemQuery     string = "query dbsystem resources"
	autonomousdbQuery string = "query autonomousdatabase resources"
	analyticsQuery    string = "query analyticsinstance resources"
	integrationQuery  string = "query integrationinstance resources"

	numWorkers int = 8
)

type Controller interface {
	SetScheduler(scheduler.Scheduler) *Controller
	Run()
}

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
func NewTagController(
	p common.ConfigurationProvider,
	tagNamespace string) (*TagController, error) {
	c := TagController{
		tagNamespace: tagNamespace,
	}
	c.log = slog.Default()
	instance, err := core.NewComputeClientWithConfigurationProvider(p)
	if err != nil {
		return nil, err
	}
	c.compute = instance

	db, err := database.NewDatabaseClientWithConfigurationProvider(p)
	if err != nil {
		return nil, err
	}
	c.database = db

	analytics, err := analytics.NewAnalyticsClientWithConfigurationProvider(p)
	if err != nil {
		return nil, err
	}
	c.analytics = analytics

	i, err := integration.NewIntegrationInstanceClientWithConfigurationProvider(p)
	if err != nil {
		return nil, err
	}
	c.integration = i

	s, err := rs.NewResourceSearchClientWithConfigurationProvider(p)
	if err != nil {
		return nil, err
	}
	c.search = s
	c.scheduler = &scheduler.NullScheduler{}

	return &c, nil
}

// SetScheduler sets the scheduler to be used for parsing run schedules
func (tc *TagController) SetScheduler(sch scheduler.Scheduler) *TagController {
	tc.scheduler = sch
	return tc
}

// Search generates a structured search and returns a resource summary collection
func (tc *TagController) Search(query string) (rs.ResourceSummaryCollection, error) {
	rsc := rs.ResourceSummaryCollection{Items: make([]rs.ResourceSummary, 0)}

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

	// Pagination
	for r, err := searchFunc(request); ; r, err = searchFunc(request) {
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

	return rsc, nil
}

// Run starts the controller spawning workers and queuing tasks
func (tc *TagController) Run() {
	tc.log.Info("Beginning TagController Run")
	var wg sync.WaitGroup

	// Search query where clause
	where := "where definedTags.Namespace = '%s'"

	// Hold resources found by search
	items := make([]rs.ResourceSummary, 0)

	// Channels for tasks and results
	tasks := make(chan task.Task, numWorkers)

	for _, q := range []string{computeQuery, dbsystemQuery, autonomousdbQuery,
		analyticsQuery, integrationQuery} {
		rsc, err := tc.Search(fmt.Sprintf(q+where, tc.tagNamespace))
		tc.log.Error("error in search",
			"error", err,
			"items returned", strconv.Itoa(len(rsc.Items)))
		items = append(items, rsc.Items...)
	}

	// Start workers
	for i := 0; i < numWorkers; i++ {
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
