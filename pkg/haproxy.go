package pkg

import (
	"os"
	"os/exec"
	"sync"
	"text/template"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/golang/glog"
)

var (
	endpointsUpdater *endpointUpdateHandler
	serviceUpdater   = &serviceUpdateHandler{}
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
	t, err := template.ParseFiles(templatePath)
	if err != nil {
		glog.Fatalf("error parsing template: %v", err)
	}
	endpointsUpdater = &endpointUpdateHandler{
		t: t,
	}
}

type endpointUpdateHandler struct {
	t *template.Template
	sync.Mutex
}

func (e *endpointUpdateHandler) OnUpdate(endpoints []api.Endpoints) {
	e.Lock()
	defer e.Unlock()
	f, err := os.Create(ConfigPath)
	if err != nil {
		glog.Errorf("error opening config file: %v", err)
		return
	}
	err = e.t.Execute(f, Convert(endpoints))
	if err != nil {
		glog.Errorf("error executing template: %v", err)
		return
	}
	cmd := exec.Command("/reload-haproxy.sh", ConfigPath)
	b, err := cmd.CombinedOutput()
	if err != nil {
		glog.Errorf("error reloading haproxy: %v", err)
		glog.Error(string(b))
		return
	}
	glog.Info("updated haproxy")
}

type serviceUpdateHandler struct {
	sync.Mutex
}

func (e *serviceUpdateHandler) OnUpdate(services []api.Service) {
}
