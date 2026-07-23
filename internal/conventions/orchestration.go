package conventions

const (
	DefaultWorkDir = "/var/www/html"
	// NodeWorkDir is the working directory inside Node-based frameworks'
	// "web" service containers (nextjs, emdash) - see their blueprint
	// compose files' working_dir/volumes.
	NodeWorkDir = "/app"
	// PythonWorkDir is the working directory inside Django's "web" service
	// container - see internal/blueprints/files/django/services.yml's
	// working_dir/volumes.
	PythonWorkDir = "/app"

	MagentoDeveloperMode  = "developer"
	MagentoProductionMode = "production"
)
