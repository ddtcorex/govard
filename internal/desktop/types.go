package desktop

type Dashboard struct {
	ActiveEnvironments int           `json:"activeEnvironments"`
	RunningServices    int           `json:"runningServices"`
	QueuedTasks        int           `json:"queuedTasks"`
	ActiveSummary      string        `json:"activeSummary"`
	ServicesSummary    string        `json:"servicesSummary"`
	QueueSummary       string        `json:"queueSummary"`
	Environments       []Environment `json:"environments"`
	Warnings           []string      `json:"warnings"`
}

type DesktopSettings struct {
	Theme              string `json:"theme"`
	ProxyTarget        string `json:"proxyTarget"`
	PreferredBrowser   string `json:"preferredBrowser"`
	CodeEditor         string `json:"codeEditor"`
	DBClientPreference string `json:"dbClientPreference"`
}

type UserInfo struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

type Service struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Port   string `json:"port"`
}

type Environment struct {
	Project        string            `json:"project"`
	Domain         string            `json:"domain"`
	ExtraDomains   []string          `json:"extraDomains,omitempty"`
	Name           string            `json:"name"`
	Framework      string            `json:"framework"`
	Technologies   []string          `json:"technologies"`
	PHP            string            `json:"php"`
	Database       string            `json:"database"`
	Services       []Service         `json:"services"`
	ServiceTargets []string          `json:"serviceTargets"`
	Status         string            `json:"status"`
	EnvVars        map[string]string `json:"envVars,omitempty"`
}

type SystemMetrics struct {
	CPUUsage    float64 `json:"cpuUsage"`
	MemoryUsage float64 `json:"memoryUsage"`
}

type RemoteSnapshot struct {
	Project  string        `json:"project"`
	Remotes  []RemoteEntry `json:"remotes"`
	Warnings []string      `json:"warnings"`
}

type RemoteEntry struct {
	Name         string   `json:"name"`
	Host         string   `json:"host"`
	User         string   `json:"user"`
	Path         string   `json:"path"`
	Port         int      `json:"port"`
	Environment  string   `json:"environment"`
	Protected    bool     `json:"protected"`
	AuthMethod   string   `json:"authMethod"`
	Capabilities []string `json:"capabilities"`
}

type RemoteUpsertInput struct {
	Name         string `json:"name"`
	Host         string `json:"host"`
	User         string `json:"user"`
	Path         string `json:"path"`
	Port         int    `json:"port"`
	Capabilities string `json:"capabilities"`
	AuthMethod   string `json:"authMethod"`
	Protected    bool   `json:"protected"`
}

type RemoteConfigSnapshot struct {
	Host         string
	User         string
	Path         string
	Port         int
	Protected    bool
	AuthMethod   string
	Capabilities []string
}
