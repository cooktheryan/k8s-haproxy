package main

import (
	"fmt"
	"os"
	"os/exec"
	"text/template"
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/meta"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/proxy/config"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"

	"github.com/golang/glog"
	flag "github.com/spf13/pflag"
)

const (
	configPath   = "/etc/haproxy/haproxy.cfg"
	templatePath = "/etc/k8s-haproxy/haproxy.cfg.gotemplate"
)

var clientConfig = &client.Config{}

func main() {
	client.BindClientConfigFlags(flag.CommandLine, clientConfig)
	flag.Set("logtostderr", "true")
	flag.Parse()

	cmd := exec.Command("haproxy", "-f", configPath, "-p", "/var/run/haproxy.pid")
	err := cmd.Run()
	if err != nil {
		if o, err := cmd.CombinedOutput(); err != nil {
			glog.Error(string(o))
		}
		glog.Fatalf("haproxy process died, : %v", err)
	}
	glog.Info("started haproxy")

	kubeClient, err := client.New(clientConfig)
	if err != nil {
		glog.Fatalf("Invalid API configuration: %v", err)
	}

	cu := configUpdater{
		make([]api.Endpoints, 0),
		make([]api.Service, 0),
		make(chan []api.Endpoints),
		make(chan []api.Service),
		template.Must(template.ParseFiles(templatePath)),
	}

	endpointsConfig := config.NewEndpointsConfig()
	serviceConfig := config.NewServiceConfig()
	endpointsConfig.RegisterHandler(cu.eu)
	serviceConfig.RegisterHandler(cu.su)

	config.NewSourceAPI(
		kubeClient.Services(api.NamespaceAll),
		kubeClient.Endpoints(api.NamespaceAll),
		30*time.Second,
		serviceConfig.Channel("api"),
		endpointsConfig.Channel("api"),
	)
	glog.Info("started watch")

	util.Forever(cu.syncLoop, 1*time.Second)
}

type configUpdater struct {
	endpoints []api.Endpoints
	services  []api.Service
	eu        endpointUpdateHandler
	su        serviceUpdateHandler
	t         *template.Template
}

func (c *configUpdater) syncLoop() {
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

func (c *configUpdater) commit() error {
	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	states, err := Convert(c.endpoints, c.services)
	if err != nil {
		return err
	}
	err = c.t.Execute(f, states)
	if err != nil {
		return err
	}
	cmd := exec.Command("/reload-haproxy.sh", configPath)
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
	Service   api.Service
	Endpoints api.Endpoints
}

func validate(s map[string]ServiceState) error {
	return nil
}

func Convert(es []api.Endpoints, ss []api.Service) (map[string]ServiceState, error) {
	sm := make(map[string]api.Service)
	em := make(map[string]api.Endpoints)
	for _, s := range ss {
		k, err := makeKey(&s)
		if err != nil {
			return nil, err
		}
		sm[k] = s
	}
	for _, e := range es {
		k, err := makeKey(&e)
		if err != nil {
			return nil, err
		}
		em[k] = e
	}
	states := make(map[string]ServiceState)
	for k, s := range sm {
		if e, found := em[k]; found {
			states[k] = ServiceState{s, e}
			continue
		}
		glog.Infof("endpoint not found for service: %v", k)
		// what should we do here?
	}
	err := validate(states)
	if err != nil {
		return nil, err
	}
	return states, nil
}

var access = meta.NewAccessor()

func makeKey(o runtime.Object) (string, error) {
	namespace, err := access.Namespace(o)
	if err != nil {
		return "", err
	}
	name, err := access.Name(o)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v-%v", namespace, name), nil
}
