package main

import (
	"os/exec"
	"text/template"

	"github.com/mikedanese/k8s-haproxy/pkg"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"

	"github.com/golang/glog"
	flag "github.com/spf13/pflag"
)

var clientConfig = &client.Config{}

const (
	configPath   = "/etc/haproxy/haproxy.cfg"
	templatePath = "/etc/k8s-haproxy/haproxy.cfg.gotemplate"
)

func init() {
	client.BindClientConfigFlags(flag.CommandLine, clientConfig)
	flag.Set("logtostderr", "true")
	flag.Parse()
}

func main() {
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

	pkg.
		NewConfigUpdater(configPath, template.Must(template.ParseFiles(templatePath))).
		Run(kubeClient)
}
