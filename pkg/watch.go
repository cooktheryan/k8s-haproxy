package pkg

import (
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/proxy/config"
)

var (
	serviceConfig   = config.NewServiceConfig()
	endpointsConfig = config.NewEndpointsConfig()
	sourceAPI       *config.SourceAPI
)

func ManageWatch(c *client.Client) {

	serviceConfig.RegisterHandler(serviceUpdater)
	endpointsConfig.RegisterHandler(endpointsUpdater)

	sourceAPI = config.NewSourceAPI(
		c.Services(api.NamespaceAll),
		c.Endpoints(api.NamespaceAll),
		30*time.Second,
		serviceConfig.Channel("api"),
		endpointsConfig.Channel("api"),
	)
}
