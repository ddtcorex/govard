package conventions

const (
	DefaultAdminUser = "admin"
	// DefaultAdminPassword is the one admin credential Govard seeds, for the
	// application admin of every framework and for Portainer alike. The value
	// is the password Portainer already carries on every installed machine
	// (bcrypt hash in internal/blueprints/files/proxy.yml), so unifying on it
	// leaves existing Portainer data volumes correct; tests/proxy_blueprint_test.go
	// keeps the two artefacts in step.
	DefaultAdminPassword = "AdminGovard123$"
	DefaultAdminEmail    = "admin@local.test"
	DefaultAdminPath     = "admin"

	DefaultDBHost = "db"
	DefaultDBUser = "app"
	DefaultDBPass = "app"
	DefaultDBName = "app"

	DefaultMagentoDBUser = "magento"
	DefaultMagentoDBPass = "magento"
	DefaultMagentoDBName = "magento"
	DefaultMagentoDBHost = DefaultDBHost

	DefaultMageOSDBUser = "mageos"
	DefaultMageOSDBPass = "mageos"
	DefaultMageOSDBName = "mageos"

	DefaultPostgresDBUser = "app"
	DefaultPostgresDBName = "app"
)
