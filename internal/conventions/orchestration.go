package conventions

const (
	DefaultWorkDir = "/var/www/html"
	// NodeWorkDir is the working directory inside Node-based frameworks'
	// "web" service containers (nextjs, emdash) - see their blueprint
	// compose files' working_dir/volumes.
	NodeWorkDir = "/app"

	MagentoDeveloperMode  = "developer"
	MagentoProductionMode = "production"
)
