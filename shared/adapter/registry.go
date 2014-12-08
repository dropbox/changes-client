package adapter

import (
	"errors"
	"fmt"
)

var (
	registry Registry
)

type Registry map[string]Adapter

func (r Registry) register(name string, adapter Adapter) {
	r[name] = adapter
}

func Register(name string, adapter Adapter) error {
	registry.register(name, adapter)
	return nil
}

func Get(name string) (Adapter, error) {
	adapter, present := registry[name]
	if present {
		return adapter, nil
	}
	return nil, errors.New(fmt.Sprintf("Adapter not found: %s", name))
}

func init() {
	registry = make(Registry)
}
