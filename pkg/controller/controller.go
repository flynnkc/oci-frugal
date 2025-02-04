package controller

import (
	"log/slog"

	"github.com/flynnkc/oci-frugal/pkg/scheduler"
	"github.com/oracle/oci-go-sdk/v65/analytics"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/integration"
)

type Controller struct {
	Scheduler   *scheduler.Scheduler
	compute     core.ComputeClient
	database    database.DatabaseClient
	analytics   analytics.AnalyticsClient
	integration integration.IntegrationInstanceClient
	log         *slog.Logger
}

func NewController(p auth.ConfigurationProviderWithClaimAccess) (*Controller, error) {
	c := Controller{}
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

	return &c, nil
}
