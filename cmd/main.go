package main

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/golang/glog"
	flag "github.com/spf13/pflag"

	"github.com/mikedanese/k8s-haproxy/pkg"
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

	pkg.ManageConfig(*templatePath)
	glog.Info("managing config")

	pkg.ManageHaproxy()
	glog.Info("managing haproxy")

	pkg.ManageWatch(kubeClient)
	glog.Info("managing watch")

	select {}
}
