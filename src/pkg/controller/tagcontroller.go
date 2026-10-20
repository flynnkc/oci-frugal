package controller

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/action"
	"github.com/flynnkc/oci-frugal/src/pkg/controller/handler"
	"github.com/flynnkc/oci-frugal/src/pkg/controller/task"
	"github.com/flynnkc/oci-frugal/src/pkg/ociapi"
	"github.com/flynnkc/oci-frugal/src/pkg/scheduler"
	"github.com/oracle/oci-go-sdk/v65/common"
	rs "github.com/oracle/oci-go-sdk/v65/resourcesearch"
)

const (
	// Keep unimplemented advertised types discoverable so they produce explicit
	// errors when tagged for an action, rather than silently disappearing.
	QUERY = "query instance, dbsystem, autonomousdatabase, " +
		"analyticsinstance, integrationinstance resources"
	TC_WORK_QUEUE = 8
)

type SearchAPI interface {
	SearchResources(
		context.Context,
		rs.SearchResourcesRequest,
	) (rs.SearchResourcesResponse, error)
	SetRegion(string)
}

type TagController struct {
	tagNamespace string
	region       string
	scheduler    scheduler.Scheduler
	action       action.Action
	handler      handler.Handler
	search       SearchAPI
	limiter      ociapi.Limiter
	close        func()
}

var _ Controller = (*TagController)(nil)

func NewTagController(opts ControllerOpts) (*TagController, error) {
	if opts.TagNamespace == nil ||
		*opts.TagNamespace == "" ||
		opts.ConfigurationProvider == nil ||
		opts.Scheduler == nil {
		return nil, ErrControllerOptions
	}
	if opts.SupportedActions != action.ALL &&
		opts.SupportedActions != action.ON &&
		opts.SupportedActions != action.OFF {
		return nil, ErrControllerOptions
	}
	limiter := ociapi.NewLimiter()
	h, err := handler.NewResourceHandler(handler.HandlerOpts{
		ConfigProvider: opts.ConfigurationProvider,
		Limiter:        limiter,
		Shutdown:       opts.Shutdown,
	})
	if err != nil {
		limiter.Close()
		return nil, err
	}
	s, err := rs.NewResourceSearchClientWithConfigurationProvider(opts.ConfigurationProvider)
	if err != nil {
		limiter.Close()
		return nil, err
	}
	c := &TagController{
		tagNamespace: *opts.TagNamespace,
		scheduler:    opts.Scheduler,
		action:       opts.SupportedActions,
		handler:      h,
		search:       &s,
		limiter:      limiter,
		close:        limiter.Close,
	}
	if opts.Region != nil {
		c.SetRegion(*opts.Region)
	}
	return c, nil
}
func (tc *TagController) Close() {
	if tc.close != nil {
		tc.close()
	}
}
func (tc *TagController) SetRegion(region string) {
	tc.region = region
	tc.search.SetRegion(region)
	tc.handler.SetRegion(region)
}

func (tc *TagController) Search(
	ctx context.Context,
	query string,
) (rs.ResourceSummaryCollection, error) {
	collection := rs.ResourceSummaryCollection{Items: make([]rs.ResourceSummary, 0)}
	req := rs.SearchResourcesRequest{
		SearchDetails: rs.StructuredSearchDetails{
			Query: common.String(query),
		},
		Limit: common.Int(1000),
	}
	pages := make(map[string]bool)
	ids := make(map[string]bool)
	for {
		resp, err := ociapi.Call(
			ctx,
			tc.limiter,
			func(ctx context.Context) (rs.SearchResourcesResponse, error) {
				return tc.search.SearchResources(ctx, req)
			},
		)
		if err != nil {
			return collection, err
		}
		for _, item := range resp.Items {
			if item.Identifier != nil {
				if ids[*item.Identifier] {
					continue
				}
				ids[*item.Identifier] = true
			}
			collection.Items = append(collection.Items, item)
		}
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		if pages[*resp.OpcNextPage] {
			return collection, fmt.Errorf("repeated resource search pagination token")
		}
		pages[*resp.OpcNextPage] = true
		req.Page = resp.OpcNextPage
	}
	return collection, nil
}

// Plan performs read-only discovery and validation. Any planning error prevents
// execution of this result. Resources without the schedule namespace are skipped.
func (tc *TagController) Plan(ctx context.Context) (Result, error) {
	result := Result{
		Region:    tc.region,
		DryRun:    true,
		StartedAt: time.Now().UTC(),
		Resources: make([]ResourceResult, 0),
	}
	collection, err := tc.Search(ctx, QUERY)
	if err != nil {
		result.Error = err.Error()
		result.FinishedAt = time.Now().UTC()
		return result, err
	}
	result.Resources = make([]ResourceResult, len(collection.Items))
	parallel(len(collection.Items), func(i int) {
		item := collection.Items[i]
		r := ResourceResult{
			ID:     value(item.Identifier),
			Type:   value(item.ResourceType),
			Action: "none",
			Status: "skipped",
		}
		defer func() { result.Resources[i] = r }()
		fail := func(err error) { r.Status = "failed"; r.Error = err.Error() }
		if err := ctx.Err(); err != nil {
			fail(err)
			return
		}
		tags, ok := item.DefinedTags[tc.tagNamespace]
		if !ok {
			return
		}
		if r.ID == "" || r.Type == "" {
			fail(fmt.Errorf("resource ID and type are required"))
			return
		}
		a, err := tc.scheduler.Evaluate(tags)
		if err != nil {
			fail(err)
			return
		}
		r.Action = a.String()
		if !action.Compare(tc.action, a) {
			return
		}
		if !handler.Supported(r.Type) {
			fail(fmt.Errorf("unsupported resource type %q", r.Type))
			return
		}
		ops, err := tc.handler.Plan(ctx, task.NewTask(a, item))
		if err != nil {
			fail(err)
			return
		}
		r.Operations = ops
		r.Status = "noop"
		for _, op := range ops {
			if op.Status == "planned" {
				r.Status = "planned"
			}
		}
	})
	err = resultErrors(result)
	if err != nil {
		result.Error = err.Error()
	}
	result.FinishedAt = time.Now().UTC()
	return result, err
}

// Execute consumes only a successful plan and rechecks state before submission.
func (tc *TagController) Execute(ctx context.Context, plan Result) (Result, error) {
	if plan.Region != tc.region {
		return plan, fmt.Errorf("plan region does not match controller")
	}
	if plan.Error != "" {
		return plan, fmt.Errorf("cannot execute failed plan: %s", plan.Error)
	}
	if err := resultErrors(plan); err != nil {
		return plan, fmt.Errorf("cannot execute failed plan: %w", err)
	}
	// Copy slices so executing does not mutate the caller's dry-run plan.
	result := plan
	result.DryRun = false
	result.Resources = append([]ResourceResult(nil), plan.Resources...)
	parallel(len(result.Resources), func(i int) {
		r := result.Resources[i]
		if r.Status != "planned" && r.Status != "noop" {
			return
		}
		r.Operations = append([]handler.Operation(nil), r.Operations...)
		var errs []error
		r.Status = "noop"
		for j, op := range r.Operations {
			// Continue after node failures so the report retains every attempted outcome.
			updated, err := tc.handler.Execute(ctx, op)
			r.Operations[j] = updated
			if err != nil {
				errs = append(errs, err)
			} else if updated.Status == "submitted" {
				r.Status = "submitted"
			}
		}
		if err := errors.Join(errs...); err != nil {
			r.Status = "failed"
			r.Error = err.Error()
		}
		result.Resources[i] = r
	})
	err := resultErrors(result)
	if err != nil {
		result.Error = err.Error()
	}
	result.FinishedAt = time.Now().UTC()
	return result, err
}

func resultErrors(result Result) error {
	var errs []error
	for _, r := range result.Resources {
		if r.Error != "" {
			errs = append(errs, fmt.Errorf("%s %s: %s", r.Type, r.ID, r.Error))
		}
	}
	return errors.Join(errs...)
}

// parallel assigns each index once and joins all workers before returning.
// Callers can write distinct preallocated slice elements without a mutex.
func parallel(n int, fn func(int)) {
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(n, TC_WORK_QUEUE) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				fn(i)
			}
		}()
	}
	for i := range n {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
}
func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
