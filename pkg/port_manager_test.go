package pkg

import (
	"fmt"
	"testing"

	"github.com/google/gofuzz"
)

func TestPortManager(t *testing.T) {
	fuzzer := fuzz.New()
	pm := newPortManager()
	for {
		spn := &ServicePortName{}
		fuzzer.Fuzz(spn)
		port := pm.Get(*spn)
		pm.Release(*spn)
		fmt.Printf("%+v %#v\n", port, *spn)
	}
}
