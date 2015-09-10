package adapter

import "fmt"

var registry = make(Registry)

type Registry map[string]func() Adapter

func (r Registry) register(name string, ctr func() Adapter) {
	r[name] = ctr
}

func Register(name string, ctr func() Adapter) error {
	registry.register(name, ctr)
	return nil
}

func Create(name string) (Adapter, error) {
	ctr, present := registry[name]
	if present {
		return ctr(), nil
	}
	return nil, fmt.Errorf("Adapter not found: %s", name)
}
