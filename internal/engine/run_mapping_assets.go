package engine

import "strings"

// RunMappingAssetPreparer prepares any framework-specific "run mapping"
// assets needed before rendering a blueprint (e.g. Magento's per-store
// nginx/apache map files), returning the nginx and apache asset paths
// ("" if this framework needs none).
type RunMappingAssetPreparer func(config Config) (nginxMapPath string, apacheMapPath string, err error)

var runMappingAssetPreparers = map[string]RunMappingAssetPreparer{}

// RegisterRunMappingAssetPreparer registers fn as the run-mapping-asset
// preparer for framework. Called from frameworks.Register so a framework
// package can own this instead of a hardcoded isMagentoFramework-style gate
// in this file. Not safe for concurrent calls; intended usage is
// registration during package init(), before PrepareRunMappingAssets is
// ever called. Not every framework needs an entry - only Magento-family
// frameworks register one.
func RegisterRunMappingAssetPreparer(framework string, fn RunMappingAssetPreparer) {
	runMappingAssetPreparers[strings.ToLower(strings.TrimSpace(framework))] = fn
}

// PrepareRunMappingAssets looks up and invokes the registered run-mapping-
// asset preparer for config.Framework, or is a no-op (empty paths, nil
// error) if none is registered - replacing the former isMagentoFramework
// gate in internal/engine/magento.go with registry-driven dispatch.
func PrepareRunMappingAssets(config Config) (string, string, error) {
	fn, ok := runMappingAssetPreparers[strings.ToLower(strings.TrimSpace(config.Framework))]
	if !ok {
		return "", "", nil
	}
	return fn(config)
}
