package reporter

import "fmt"

var registry = make(Registry)

type Registry map[string]func() Reporter

func (r Registry) register(name string, ctr func() Reporter) error {
	r[name] = ctr
	return nil
}

func (r Registry) names() []string {
    var res []string
    for k := range r {
        res = append(res, k)
    }
    return res
}

func Register(name string, ctr func() Reporter) error {
	return registry.register(name, ctr)
}

// Names returns the names of all registered Reporters.
func Names() []string {
    return registry.names()
}


func Create(name string) (Reporter, error) {
	ctr, present := registry[name]
	if present {
		return ctr(), nil
	}
	return nil, fmt.Errorf("Reporter not found: %s", name)
}
