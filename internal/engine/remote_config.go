package engine

import (
	"gopkg.in/yaml.v3"
	"sort"
	"strings"
)

const (
	RemoteAuthMethodSSHAgent = "ssh-agent"
	RemoteAuthMethodKeychain = "keychain"
	RemoteAuthMethodKeyfile  = "keyfile"
)

type RemoteAuth struct {
	Method         string `yaml:"method,omitempty"`
	KeyPath        string `yaml:"key_path,omitempty"`
	StrictHostKey  bool   `yaml:"strict_host_key,omitempty"`
	KnownHostsFile string `yaml:"known_hosts_file,omitempty"`
}

type RemotePaths struct {
	Media string `yaml:"media,omitempty"`
}

type RemoteCapabilities struct {
	Files *bool `yaml:"files,omitempty"`
	Media *bool `yaml:"media,omitempty"`
	DB    *bool `yaml:"db,omitempty"`
}

type RemoteConfig struct {
	Host         string              `yaml:"host"`
	User         string              `yaml:"user"`
	Port         int                 `yaml:"port"`
	Path         string              `yaml:"path"`
	URL          string              `yaml:"url,omitempty"`
	Protected    *bool               `yaml:"protected,omitempty"`
	Capabilities *RemoteCapabilities `yaml:"capabilities,omitempty"`
	Auth         RemoteAuth          `yaml:"auth,omitempty"`
	Paths        RemotePaths         `yaml:"paths,omitempty"`
	DBName       string              `yaml:"db_name,omitempty"`
	DBUser       string              `yaml:"db_user,omitempty"`
	DBPass       string              `yaml:"db_pass,omitempty"`
	DBPort       int                 `yaml:"db_port,omitempty"`
}

// RemoteConfigMap is a specialized map that preserves sort order during YAML marshaling.
type RemoteConfigMap map[string]RemoteConfig

// MarshalYAML implements the yaml.Marshaler interface to ensure remotes are written to
// .govard.yml in a consistent priority order (dev => staging => prod).
func (m RemoteConfigMap) MarshalYAML() (interface{}, error) {
	if m == nil {
		return nil, nil
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	SortRemoteNames(keys)

	node := &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}

	for _, k := range keys {
		v := m[k]

		// Key node
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: k,
		})

		// Value node
		valNode := &yaml.Node{}
		if err := valNode.Encode(v); err != nil {
			return nil, err
		}
		node.Content = append(node.Content, valNode)
	}

	return node, nil
}

// BoolPtr returns a pointer to a bool value, for use with RemoteConfig.Protected.
func BoolPtr(v bool) *bool {
	return &v
}

func NormalizeRemoteAuthMethod(method string) string {
	normalized := strings.ToLower(strings.TrimSpace(method))
	switch normalized {
	case "":
		return RemoteAuthMethodKeychain
	case "ssh-agent", "ssh_agent", "sshagent":
		return RemoteAuthMethodSSHAgent
	case "keychain":
		return RemoteAuthMethodKeychain
	case "keyfile", "key-file":
		return RemoteAuthMethodKeyfile
	default:
		return normalized
	}
}

func IsSupportedRemoteAuthMethod(method string) bool {
	switch NormalizeRemoteAuthMethod(method) {
	case RemoteAuthMethodSSHAgent, RemoteAuthMethodKeychain, RemoteAuthMethodKeyfile:
		return true
	default:
		return false
	}
}

// RemotePriority returns a priority number for sorting remotes.
// Smaller numbers mean higher priority (earlier in the list).
func remotePriority(name string) int {
	switch NormalizeRemoteEnvironment(name) {
	case RemoteEnvDev:
		return 10
	case RemoteEnvStaging:
		return 20
	case RemoteEnvProd:
		return 30
	default:
		return 100
	}
}

// SortRemoteNames sorts a slice of remote names based on RemotePriority,
// then alphabetically for names with equal priority.
func SortRemoteNames(names []string) {
	sort.Slice(names, func(i, j int) bool {
		pi := remotePriority(names[i])
		pj := remotePriority(names[j])
		if pi != pj {
			return pi < pj
		}
		return names[i] < names[j]
	})
}
