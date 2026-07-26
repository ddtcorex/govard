package prestashop

import "govard/internal/engine"

// manifest is PrestaShop's FrameworkManifestConfig literal - registered
// into engine's runtime store by Definition() (via frameworks.Register),
// replacing the old "prestashop" entry that used to live in
// internal/engine/framework_manifest.json's "frameworks" object.
var manifest = engine.FrameworkManifestConfig{
	Ignored: []string{
		"connections",
		"connections_page",
		"connections_source",
		"guest",
		"statssearch",
		"search_index",
		"search_word",
		"log",
		"mail",
	},
	Sensitive: []string{
		"customer",
		"address",
		"orders",
		"order_detail",
		"order_payment",
		"order_invoice",
		"order_return",
		"cart",
		"customer_thread",
		"customer_message",
		"employee",
		"wishlist",
		"wishlist_product",
	},
	Paths: engine.FrameworkPathConfig{
		LocalMedia:        "img",
		RemoteMedia:       "img",
		WebRootCandidates: []engine.FrameworkWebRootCandidate{},
	},
	Sync: engine.FrameworkSyncConfig{
		NoiseExcludes: []string{"var/cache/", "var/logs/", "log/"},
		MediaExcludes: engine.FrameworkMediaExcludeSet{
			NonAll:    []string{},
			Optimized: []string{},
			Minimal:   []string{},
		},
	},
	Features: engine.FrameworkFeatureConfig{
		RequiresRunningEnvForFreshInstall: true,
		SupportsPostClone:                 true,
	},
}
