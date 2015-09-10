package reporter

import "fmt"

var registry = make(Registry)

type Registry map[string]func() Reporter

func (r Registry) register(name string, ctr func() Reporter) error {
	r[name] = ctr
	return nil
}

func Register(name string, ctr func() Reporter) error {
	return registry.register(name, ctr)
}

func Create(name string) (Reporter, error) {
	ctr, present := registry[name]
	if present {
		return ctr(), nil
	}
	return nil, fmt.Errorf("Reporter not found: %s", name)
}
