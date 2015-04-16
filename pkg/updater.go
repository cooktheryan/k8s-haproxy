package pkg

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"text/template"
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/proxy/config"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/types"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/golang/glog"
)

type ConfigUpdater struct {
	configPath string
	endpoints  []api.Endpoints
	services   []api.Service
	eu         endpointUpdateHandler
	su         serviceUpdateHandler
	ports      *portManager
	t          *template.Template
}

func NewConfigUpdater(configPath string, t *template.Template) *ConfigUpdater {
	return &ConfigUpdater{
		configPath,
		make([]api.Endpoints, 0),
		make([]api.Service, 0),
		make(chan []api.Endpoints),
		make(chan []api.Service),
		newPortManager(),
		t,
	}
}

func (cu *ConfigUpdater) Run(c *client.Client) {
	endpointsConfig := config.NewEndpointsConfig()
	serviceConfig := config.NewServiceConfig()
	endpointsConfig.RegisterHandler(cu.eu)
	serviceConfig.RegisterHandler(cu.su)

	config.NewSourceAPI(
		c.Services(api.NamespaceAll),
		c.Endpoints(api.NamespaceAll),
		30*time.Second,
		serviceConfig.Channel("api"),
		endpointsConfig.Channel("api"),
	)
	glog.Info("started watch")

	util.Forever(cu.syncLoop, 1*time.Second)
}

func (c *ConfigUpdater) syncLoop() {
	for {
		select {
		case sl := <-c.su:
			c.services = sl
			break
		case el := <-c.eu:
			c.endpoints = el
			break
		}

		if err := c.commit(); err != nil {
			glog.Errorf("error commiting haproxy config: %v", err)
			continue
		}
		glog.Info("updated haproxy")
	}
}

func (c *ConfigUpdater) commit() error {
	f, err := os.Create(c.configPath)
	if err != nil {
		return err
	}
	states, err := c.Convert(c.endpoints, c.services)
	if err != nil {
		return err
	}
	err = c.t.Execute(f, states)
	if err != nil {
		return err
	}
	cmd := exec.Command("/reload-haproxy.sh", c.configPath)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error reloading haproxy: %v: %v", err, string(b))
	}
	return nil
}

type endpointUpdateHandler chan []api.Endpoints

func (h endpointUpdateHandler) OnUpdate(newEndpoints []api.Endpoints) {
	h <- newEndpoints
}

type serviceUpdateHandler chan []api.Service

func (h serviceUpdateHandler) OnUpdate(newServices []api.Service) {
	h <- newServices
}

type ServiceState struct {
	EphemeralPorts map[ServicePortName]int
	Service        api.Service
	Endpoints      api.Endpoints
}

type ServiceMapping struct {
	EphemeralPort int
	Address       []api.EndpointAddress
}

func validate(s map[types.NamespacedName]ServiceState) error {
	return nil
}

func (c *ConfigUpdater) Convert(es []api.Endpoints, ss []api.Service) (map[types.NamespacedName]ServiceState, error) {
	sm := make(map[types.NamespacedName]api.Service)
	em := make(map[types.NamespacedName]api.Endpoints)
	for _, s := range ss {
		sm[types.NamespacedName{Namespace: s.Namespace, Name: s.Name}] = s
	}
	for _, e := range es {
		em[types.NamespacedName{Namespace: e.Namespace, Name: e.Name}] = e
	}
	states := make(map[types.NamespacedName]ServiceState)
	for k, s := range sm {
		e, found := em[k]
		if !found {
			// what should we do here?
			glog.Infof("endpoint not found for service: %v", k)
			continue
		}
		ephemeralPorts := make(map[ServicePortName]int)
		for _, port := range s.Spec.Ports {
			spn := ServicePortName{k, strconv.Itoa(port.Port)}
			ephemeralPorts[spn] = c.ports.Get(spn)
		}
		states[k] = ServiceState{
			EphemeralPorts: ephemeralPorts,
			Service:        s,
			Endpoints:      e,
		}
	}
	err := validate(states)
	if err != nil {
		return nil, err
	}
	return states, nil
}
