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
	Theme            string `json:"theme"`
	ProxyTarget      string `json:"proxyTarget"`
	PreferredBrowser string `json:"preferredBrowser"`
	CodeEditor       string `json:"codeEditor"`
}

type UserInfo struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

type Environment struct {
	Project        string   `json:"project"`
	Domain         string   `json:"domain"`
	Name           string   `json:"name"`
	Framework      string   `json:"framework"`
	PHP            string   `json:"php"`
	Database       string   `json:"database"`
	Services       []string `json:"services"`
	ServiceTargets []string `json:"serviceTargets"`
	Status         string   `json:"status"`
}

type ResourceMetricsSnapshot struct {
	UpdatedAt    string                  `json:"updatedAt"`
	SystemCPU    float64                 `json:"systemCPU"`
	SystemMemory float64                 `json:"systemMemory"`
	Summary      ResourceMetricsSummary  `json:"summary"`
	Projects     []ProjectResourceMetric `json:"projects"`
	Warnings     []string                `json:"warnings"`
}

type SystemMetrics struct {
	CPUUsage    float64 `json:"cpuUsage"`
	MemoryUsage float64 `json:"memoryUsage"`
}

type ResourceMetricsSummary struct {
	ActiveProjects int     `json:"activeProjects"`
	CPUPercent     float64 `json:"cpuPercent"`
	MemoryMB       float64 `json:"memoryMB"`
	NetRxMB        float64 `json:"netRxMB"`
	NetTxMB        float64 `json:"netTxMB"`
	OOMProjects    int     `json:"oomProjects"`
}

type ProjectResourceMetric struct {
	Project       string  `json:"project"`
	Status        string  `json:"status"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryMB      float64 `json:"memoryMB"`
	MemoryPercent float64 `json:"memoryPercent"`
	NetRxMB       float64 `json:"netRxMB"`
	NetTxMB       float64 `json:"netTxMB"`
	OOMKilled     bool    `json:"oomKilled"`
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
	Environment  string `json:"environment"`
	Capabilities string `json:"capabilities"`
	AuthMethod   string `json:"authMethod"`
	Protected    bool   `json:"protected"`
}

type RemoteConfigSnapshot struct {
	Host         string
	User         string
	Path         string
	Port         int
	Environment  string
	Protected    bool
	AuthMethod   string
	Capabilities []string
}
