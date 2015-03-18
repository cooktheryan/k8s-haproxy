package main

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"text/template"
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/meta"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/proxy/config"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"

	"github.com/golang/glog"
	flag "github.com/spf13/pflag"
)

var (
	templatePath = flag.String("template_path", "/etc/k8s-haproxy/haproxy.cfg.gotemplate", "location of the haproxy template")

	clientConfig    = &client.Config{}
	serviceConfig   = config.NewServiceConfig()
	endpointsConfig = config.NewEndpointsConfig()
	sourceAPI       *config.SourceAPI

	endpointsUpdater *endpointUpdateHandler
	serviceUpdater   = &serviceUpdateHandler{}

	t    *template.Template
	lock sync.Mutex

	endpoints = []api.Endpoints{}
	services  = []api.Service{}
)

const ConfigPath = "/etc/haproxy/haproxy.cfg"

func init() {
	client.BindClientConfigFlags(flag.CommandLine, clientConfig)
	flag.Set("logtostderr", "true")
	flag.Parse()
	t = template.Must(template.ParseFiles(*templatePath))
}

func main() {
	cmd := exec.Command("haproxy", "-f", ConfigPath, "-p", "/var/run/haproxy.pid")
	err := cmd.Run()
	if err != nil {
		if o, err := cmd.CombinedOutput(); err != nil {
			glog.Error(string(o))
		}
		glog.Errorf("haproxy process died, : %v", err)
	}
	glog.Info("started haproxy")

	kubeClient, err := client.New(clientConfig)
	if err != nil {
		glog.Fatalf("Invalid API configuration: %v", err)
	}
	serviceConfig.RegisterHandler(serviceUpdater)
	endpointsConfig.RegisterHandler(endpointsUpdater)

	sourceAPI = config.NewSourceAPI(
		kubeClient.Services(api.NamespaceAll),
		kubeClient.Endpoints(api.NamespaceAll),
		30*time.Second,
		serviceConfig.Channel("api"),
		endpointsConfig.Channel("api"),
	)
	glog.Info("started watch")

	select {}
}

type endpointUpdateHandler struct{}

func (e *endpointUpdateHandler) OnUpdate(newEndpoints []api.Endpoints) {
	lock.Lock()
	endpoints = newEndpoints
	lock.Unlock()
	err := Commit()
	if err != nil {
		glog.Errorf("error commiting haproxy config: %v", err)
	}
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
	states, err := Convert(endpoints, services)
	if err != nil {
		return err
	}
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
		glog.Infof("endpoint not found for service: %+v", s)
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
