package engine

import "strings"

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
	Files  bool `yaml:"files"`
	Media  bool `yaml:"media"`
	DB     bool `yaml:"db"`
	Deploy bool `yaml:"deploy"`
}

type RemoteConfig struct {
	Host         string             `yaml:"host"`
	User         string             `yaml:"user"`
	Port         int                `yaml:"port"`
	Path         string             `yaml:"path"`
	URL          string             `yaml:"url,omitempty"`
	Environment  string             `yaml:"environment"`
	Protected    bool               `yaml:"protected"`
	Capabilities RemoteCapabilities `yaml:"capabilities"`
	Auth         RemoteAuth         `yaml:"auth,omitempty"`
	Paths        RemotePaths        `yaml:"paths,omitempty"`
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
