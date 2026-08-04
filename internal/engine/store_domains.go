package engine

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type StoreDomainMapping struct {
	Code string `yaml:"code,omitempty"`
	Type string `yaml:"type,omitempty"`
}

type StoreDomainMappings map[string]StoreDomainMapping

func (m *StoreDomainMapping) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.ScalarNode:
		var code string
		if err := node.Decode(&code); err != nil {
			return err
		}
		m.Code = strings.TrimSpace(code)
		m.Type = ""
		return nil
	case yaml.MappingNode:
		type rawStoreDomainMapping StoreDomainMapping
		var raw rawStoreDomainMapping
		if err := node.Decode(&raw); err != nil {
			return err
		}
		m.Code = strings.TrimSpace(raw.Code)
		m.Type = normalizeStoreDomainType(raw.Type)
		return nil
	default:
		return fmt.Errorf("store domain mapping must be a string or mapping")
	}
}

func (m StoreDomainMapping) ScopeCode() string {
	return strings.TrimSpace(m.Code)
}

func (m StoreDomainMapping) ScopeType() string {
	return normalizeStoreDomainType(m.Type)
}

func normalizeStoreDomainType(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeStoreDomainMappings(mappings StoreDomainMappings) StoreDomainMappings {
	if len(mappings) == 0 {
		return mappings
	}

	normalized := make(StoreDomainMappings, len(mappings))
	for host, mapping := range mappings {
		trimmedHost := strings.TrimSpace(host)
		if trimmedHost == "" {
			continue
		}
		normalized[trimmedHost] = StoreDomainMapping{
			Code: strings.TrimSpace(mapping.Code),
			Type: normalizeStoreDomainType(mapping.Type),
		}
	}
	return normalized
}

func SortedStoreDomainHosts(mappings StoreDomainMappings) []string {
	hosts := make([]string, 0, len(mappings))
	for host := range mappings {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts
}
