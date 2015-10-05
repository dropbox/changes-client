package adapter

import "fmt"

var registry = make(Registry)

type Registry map[string]func() Adapter

func (r Registry) register(name string, ctr func() Adapter) {
	r[name] = ctr
}

func (r Registry) names() []string {
	var res []string
	for k := range r {
		res = append(res, k)
	}
	return res
}

func Register(name string, ctr func() Adapter) error {
	registry.register(name, ctr)
	return nil
}

// Names returns the names of all registered Adapters.
func Names() []string {
	return registry.names()
}

func Create(name string) (Adapter, error) {
	ctr, present := registry[name]
	if present {
		return ctr(), nil
	}
	return nil, fmt.Errorf("Adapter not found: %s", name)
}
