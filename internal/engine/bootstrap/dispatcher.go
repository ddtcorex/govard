package bootstrap

import "fmt"

func Run(framework string, opts Options) error {
	switch framework {
	case "magento2":
		return BootstrapMagento2(opts)
	case "magento1":
		return BootstrapMagento1(opts)
	case "openmage":
		return BootstrapOpenMage(opts)
	case "laravel":
		return BootstrapLaravel(opts)
	case "symfony":
		return BootstrapSymfony(opts)
	case "drupal":
		return BootstrapDrupal(opts)
	case "wordpress":
		return BootstrapWordPress(opts)
	case "nextjs":
		return BootstrapNextJS(opts)
	case "emdash":
		return BootstrapEmdash(opts)
	case "shopware":
		return BootstrapShopware(opts)
	case "cakephp":
		return BootstrapCakePHP(opts)
	case "prestashop":
		return BootstrapPrestaShop(opts)
	default:
		return fmt.Errorf("unsupported framework: %s", framework)
	}
}

func BootstrapMagento2(opts Options) error {
	_ = Magento2FreshCommands(opts)
	return nil
}

func BootstrapMagento1(opts Options) error {
	bootstrap := NewMagento1Bootstrap(opts)
	_ = bootstrap.FreshCommands()
	return nil
}

func BootstrapOpenMage(opts Options) error {
	bootstrap := NewOpenMageBootstrap(opts)
	_ = bootstrap.FreshCommands()
	return nil
}

func BootstrapLaravel(opts Options) error {
	bootstrap := NewLaravelBootstrap(opts)
	_ = bootstrap.FreshCommands()
	return nil
}

func BootstrapSymfony(opts Options) error {
	bootstrap := NewSymfonyBootstrap(opts)
	_ = bootstrap.FreshCommands()
	return nil
}

func BootstrapDrupal(opts Options) error {
	bootstrap := NewDrupalBootstrap(opts)
	_ = bootstrap.FreshCommands()
	return nil
}

func BootstrapWordPress(opts Options) error {
	bootstrap := NewWordPressBootstrap(opts)
	_ = bootstrap.FreshCommands()
	return nil
}

func BootstrapNextJS(opts Options) error {
	bootstrap := NewNextJSBootstrap(opts)
	_ = bootstrap.FreshCommands()
	return nil
}

func BootstrapEmdash(opts Options) error {
	bootstrap := NewEmdashBootstrap(opts)
	_ = bootstrap.FreshCommands()
	return nil
}

func BootstrapShopware(opts Options) error {
	bootstrap := NewShopwareBootstrap(opts)
	_ = bootstrap.FreshCommands()
	return nil
}

func BootstrapCakePHP(opts Options) error {
	bootstrap := NewCakePHPBootstrap(opts)
	_ = bootstrap.FreshCommands()
	return nil
}

func BootstrapPrestaShop(opts Options) error {
	bootstrap := NewPrestaShopBootstrap(opts)
	_ = bootstrap.FreshCommands()
	return nil
}
