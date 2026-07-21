package bootstrap

type Options struct {
	Source   string
	Clone    bool
	CodeOnly bool
	Fresh    bool
	Version  string
	Env      string
	Runner   func(command string) error

	// Database credentials for local configuration
	DBHost      string
	DBUser      string
	DBPass      string
	DBName      string
	TablePrefix string

	// Environment configuration
	Domain      string
	ProjectName string

	// PrestaShop encryption secrets carried over from a remote's parameters.php, so a
	// fabricated local parameters.php can reuse them instead of generating fresh ones
	// (module data encrypted under the remote's keys would otherwise be undecryptable
	// after a DB clone). Left empty when no remote secrets were available/probed.
	PrestaShopSecret       string
	PrestaShopCookieKey    string
	PrestaShopCookieIV     string
	PrestaShopNewCookieKey string
}

func DefaultOptions() Options {
	return Options{Source: "staging"}
}

type FrameworkBootstrap interface {
	Name() string
	SupportsFreshInstall() bool
	SupportsClone() bool
	FreshCommands() []string
	CreateProject(projectDir string) error
	Install(projectDir string) error
	Configure(projectDir string) error
	PostClone(projectDir string) error
}

func Magento2FreshCommands(opts Options) []string {
	version := opts.Version
	if version == "" {
		version = "2.4.8"
	}
	return []string{
		"composer create-project magento/project-community-edition:" + version + " .",
	}
}

func MageOSFreshCommands(opts Options) []string {
	version := opts.Version
	if version == "" {
		version = "1.3.1"
	}
	return []string{
		"composer create-project mage-os/project-community-edition:" + version + " --repository-url=https://repo.mage-os.org .",
	}
}
