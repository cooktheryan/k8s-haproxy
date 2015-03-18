package main

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"text/template"
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/proxy/config"

	"github.com/golang/glog"
	flag "github.com/spf13/pflag"
)

var (
	clientConfig = &client.Config{}
	templatePath = flag.String("template_path", "/etc/k8s-haproxy/haproxy.cfg.gotemplate", "location of the haproxy template")
)

func init() {
	client.BindClientConfigFlags(flag.CommandLine, clientConfig)
	flag.Set("logtostderr", "true")
}

func main() {
	flag.Parse()

	kubeClient, err := client.New(clientConfig)
	if err != nil {
		glog.Fatalf("Invalid API configuration: %v", err)
	}

	ManageConfig(*templatePath)
	glog.Info("managing config")

	ManageHaproxy()
	glog.Info("managing haproxy")

	ManageWatch(kubeClient)
	glog.Info("managing watch")

	select {}
}

//haproxy stuff
var (
	endpointsUpdater *endpointUpdateHandler
	serviceUpdater   = &serviceUpdateHandler{}

	t    *template.Template
	lock sync.Mutex

	endpoints = []api.Endpoints{}
	services  = []api.Service{}
)

const ConfigPath = "/etc/haproxy/haproxy.cfg"

func ManageHaproxy() {
	cmd := exec.Command("haproxy", "-f", ConfigPath, "-p", "/var/run/haproxy.pid")
	err := cmd.Run()
	if err != nil {
		if o, err := cmd.CombinedOutput(); err != nil {
			glog.Error(string(o))
		}
		glog.Errorf("haproxy process died, : %v", err)
	}
}

func ManageConfig(templatePath string) {
	var err error
	t, err = template.ParseFiles(templatePath)
	if err != nil {
		glog.Fatalf("error parsing template: %v", err)
	}
}

type endpointUpdateHandler struct{}

func (e *endpointUpdateHandler) OnUpdate(newEndpoints []api.Endpoints) {
	lock.Lock()
	endpoints = newEndpoints
	lock.Unlock()
	Commit()
}

type serviceUpdateHandler struct{}

func (e *serviceUpdateHandler) OnUpdate(newServices []api.Service) {
	lock.Lock()
	services = newServices
	lock.Unlock()
	err := Commit()
	if err != nil {
		glog.Errorf("error commiting haproxy config: %v", err)
	}
}

func Commit() error {
	lock.Lock()
	defer lock.Unlock()
	f, err := os.Create(ConfigPath)
	if err != nil {
		return err
	}
	states := Convert(endpoints, services)
	err = validate(states)
	if err != nil {
		return err
	}
	err = t.Execute(f, states)
	if err != nil {
		return err
	}
	cmd := exec.Command("/reload-haproxy.sh", ConfigPath)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error reloading haproxy: %v: %v", err, string(b))
	}
	glog.Info("updated haproxy")
	return nil
}

type ServiceState struct {
	Service   api.Service
	Endpoints api.Endpoints
}

func validate(s map[string]ServiceState) error {
	return nil
}

func Convert(es []api.Endpoints, ss []api.Service) map[string]ServiceState {
	sm := make(map[string]api.Service)
	em := make(map[string]api.Endpoints)
	for _, s := range ss {
		sm[s.Name] = s
	}
	for _, e := range es {
		em[e.Name] = e
	}
	states := make(map[string]ServiceState)
	for k, s := range sm {
		if e, found := em[k]; found {
			states[k] = ServiceState{s, e}
			continue
		}
		glog.Infof("endpoint not found for service: %+v", s)
		// what should we do here?
	}
	return states
}

//watch stuff
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
