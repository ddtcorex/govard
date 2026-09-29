package magento2

import (
	"os"
	"path/filepath"
	"strings"
)

// appCodeDir is where Magento registers in-app modules: app/code/<Vendor>/
// <Module>/etc/module.xml, two levels deep — the layout the audit's
// module_in_project mode resolves from.
const appCodeDir = "app/code"

// AuditModuleDir returns the first in-app module below app/code, in directory
// order, or false when the project carries none. The `govard verify` rows for
// the audit's module-scoped modes audit exactly one module; picking the first in
// sorted order keeps an item that runs twice on the same project on the same
// directory.
//
// A vendor or module entry is stat'ed rather than read from the directory entry
// so a symlinked tree still resolves. An unreadable directory contributes
// nothing instead of an error: this hook's contract is (string, bool), and the
// item's verified alternative to a discovered module is a skip.
func AuditModuleDir(projectRoot string) (string, bool) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return "", false
	}

	appCode := filepath.Join(root, filepath.FromSlash(appCodeDir))
	vendors, err := os.ReadDir(appCode)
	if err != nil {
		return "", false
	}
	for _, vendor := range vendors {
		vendorDir := filepath.Join(appCode, vendor.Name())
		if !isDirectory(vendorDir) {
			continue
		}
		modules, err := os.ReadDir(vendorDir)
		if err != nil {
			continue
		}
		for _, module := range modules {
			moduleDir := filepath.Join(vendorDir, module.Name())
			if !isDirectory(moduleDir) {
				continue
			}
			declared, err := hasMagentoModuleDeclaration(moduleDir)
			if err != nil || !declared {
				continue
			}
			return moduleDir, true
		}
	}
	return "", false
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
