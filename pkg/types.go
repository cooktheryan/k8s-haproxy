package pkg

import (
	"fmt"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
)

type Service struct {
	Name     string
	Port     int
	Backends []string
}

func Convert(e []api.Endpoints) []Service {
	services := make([]Service, 0)
	for _, e := range e {
		if len(e.Endpoints) > 0 {
			bs := make([]string, 0)
			for _, b := range e.Endpoints {
				bs = append(bs, b.IP)
			}
			var name string
			if len(e.ObjectMeta.Namespace) > 0 {
				name = fmt.Sprintf("%v-%v", e.ObjectMeta.Namespace, e.ObjectMeta.Name)
			} else {
				name = e.ObjectMeta.Name
			}
			services = append(services, Service{
				Name:     name,
				Port:     e.Endpoints[0].Port,
				Backends: bs,
			})
		}
	}
	return services
}
