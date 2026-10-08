package conventions

const (
	HTTPPort         = 80
	MySQLPort        = 3306
	PostgresPort     = 5432
	PHPFPMPort       = 9000
	RedisPort        = 6379
	SearchPort       = 9200
	RabbitMQPort     = 5672
	RabbitMQMgmtPort = 15672
	// SMTPPort is the port of the shared mail catcher.
	SMTPPort = 1025
)

// DefaultMailHost is the hostname under which the shared mail catcher is
// reachable from inside a project's PHP container (the govard-proxy network
// alias of the mail service). The mailpit image name does not resolve there.
const DefaultMailHost = "mail"
