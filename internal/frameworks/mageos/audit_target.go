package mageos

import (
	"encoding/json"
	"os"
	"path/filepath"

	"govard/internal/frameworks/magento2"
	"govard/internal/frameworks/types"
)

var mageOSAuditPackages = map[string]struct{}{
	"mage-os/product-community-edition": {},
	"mage-os/project-community-edition": {},
}

// ResolveAuditTarget claims Magento targets only when their project root has
// Mage-OS package evidence. Generic Magento targets remain owned by the
// inherited Magento 2 resolver.
func ResolveAuditTarget(request types.AuditTargetResolveRequest) (types.AuditTarget, bool, error) {
	target, recognized, err := magento2.ResolveAuditTarget(request)
	if err != nil || !recognized || target.ProjectRoot == "" {
		return target, false, err
	}
	isMageOS, err := isMageOSAuditProject(target.ProjectRoot)
	if err != nil {
		return types.AuditTarget{}, false, err
	}
	if !isMageOS {
		return types.AuditTarget{}, false, nil
	}
	return target, true, nil
}

func isMageOSAuditProject(root string) (bool, error) {
	marker, err := os.Lstat(filepath.Join(root, magento2.BinMagento))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !marker.Mode().IsRegular() {
		return false, nil
	}
	contents, err := os.ReadFile(filepath.Join(root, "composer.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var manifest struct {
		Require map[string]json.RawMessage `json:"require"`
	}
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return false, err
	}
	for packageName := range manifest.Require {
		if _, ok := mageOSAuditPackages[packageName]; ok {
			return true, nil
		}
	}
	return false, nil
}
