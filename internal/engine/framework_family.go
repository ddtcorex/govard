package engine

import "strings"

// IsMagento2Family reports whether framework uses Magento 2-compatible
// commands, configuration, and runtime behavior. Mage-OS is a drop-in fork
// of Magento 2 and therefore belongs to this family.
func IsMagento2Family(framework string) bool {
	switch strings.ToLower(strings.TrimSpace(framework)) {
	case "magento2", "magento", "mageos":
		return true
	default:
		return false
	}
}

// Magento2FamilyDisplayName returns the user-facing name for the framework
// distribution. Callers should first ensure the framework is in the Magento 2
// family with IsMagento2Family.
func Magento2FamilyDisplayName(framework string) string {
	if strings.EqualFold(strings.TrimSpace(framework), "mageos") {
		return "Mage-OS"
	}
	return "Magento 2"
}
