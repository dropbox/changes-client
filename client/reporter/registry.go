package reporter

import (
	"errors"
	"fmt"
)

var (
	registry Registry
)

type Registry map[string]Reporter

func (r Registry) register(name string, reporter Reporter) {
	r[name] = reporter
}

func Register(name string, reporter Reporter) error {
	registry.register(name, reporter)
	return nil
}

func Get(name string) (Reporter, error) {
	reporter, present := registry[name]
	if present {
		return reporter, nil
	}
	return nil, errors.New(fmt.Sprintf("Reporter not found: %s", name))
}

func init() {
	registry = make(Registry)
}
