package pkg

import "sync"

func newPortManager() *portManager {
	return &portManager{
		i: 0,
		p: make(map[ServicePortName]int),
	}
}

type portManager struct {
	sync.RWMutex
	i int
	p map[ServicePortName]int
}

func (pm *portManager) Get(spn ServicePortName) int {
	pm.RLock()
	defer pm.RUnlock()
	out, found := pm.p[spn]
	if !found {
		out = pm.findOpenPort()
		pm.p[spn] = out
	}
	return out

}

func (pm *portManager) Release(spn ServicePortName) {
	pm.Lock()
	defer pm.Unlock()
	delete(pm.p, spn)
}

func (pm *portManager) findOpenPort() int {
	pm.i += 1
	toReturn := 40000 + pm.i%20000
	for _, used := range pm.p {
		if used == toReturn {
			return pm.findOpenPort()
		}
	}
	return toReturn
}
