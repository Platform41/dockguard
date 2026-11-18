package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func parseDependencies(node *yaml.Node) ([]string, error) {
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value != "depends_on" {
			continue
		}
		list := node.Content[i+1]
		if list.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("depends_on must be a list of target names")
		}
		names := make([]string, 0, len(list.Content))
		for _, item := range list.Content {
			if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
				return nil, fmt.Errorf("depends_on entries must be strings")
			}
			names = append(names, item.Value)
		}
		return names, nil
	}
	return nil, nil
}

func validateDependencies(cfg Config) error {
	hasSystem := false
	for _, t := range cfg.Targets {
		hasSystem = hasSystem || t.Kind == KindContainerSystem
	}
	for _, t := range cfg.Targets {
		if hasSystem && t.Container != nil && t.Container.StopSystem {
			return fmt.Errorf("target %q: stop_system conflicts with a container-system target; use depends_on instead", t.Name)
		}
		seen := map[string]bool{}
		for _, name := range t.DependsOn {
			if name == t.Name {
				return fmt.Errorf("target %q: depends_on cannot reference itself", t.Name)
			}
			if seen[name] {
				return fmt.Errorf("target %q: depends_on lists %q twice", t.Name, name)
			}
			seen[name] = true
			if _, ok := cfg.Find(name); !ok {
				return fmt.Errorf("target %q: depends_on references unknown target %q", t.Name, name)
			}
		}
	}
	state := map[string]int{}
	var path []string
	var visit func(string) error
	visit = func(name string) error {
		if state[name] == 2 {
			return nil
		}
		if state[name] == 1 {
			for i, n := range path {
				if n == name {
					return fmt.Errorf("target %q: dependency cycle: %s", name, strings.Join(append(append([]string{}, path[i:]...), name), " → "))
				}
			}
		}
		state[name] = 1
		path = append(path, name)
		t, _ := cfg.Find(name)
		for _, dep := range t.DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		path = path[:len(path)-1]
		state[name] = 2
		return nil
	}
	for _, t := range cfg.Targets {
		if err := visit(t.Name); err != nil {
			return err
		}
	}
	return nil
}

func parseContainerSystem(name string, node *yaml.Node) (Target, error) {
	var keys struct {
		DataPath             string `yaml:"data_path"`
		FailIfAlreadyRunning *bool  `yaml:"fail_if_already_running"`
	}
	if err := node.Decode(&keys); err != nil {
		return Target{}, err
	}
	path := cleanPath(keys.DataPath)
	if err := requireAbsolute("data_path", path); err != nil {
		return Target{}, err
	}
	return Target{Name: name, Kind: KindContainerSystem, FailIfAlreadyRunning: boolOr(keys.FailIfAlreadyRunning, true), ContainerSystem: &ContainerSystem{DataPath: path}}, nil
}
