package engine

import "strings"

var migrationFrameworks = map[string]map[string]string{}

// RegisterMigrationFramework associates an external migration-tool type with
// a framework's canonical name. Framework packages register these mappings at
// startup; engine keeps only generic source/type lookup.
func RegisterMigrationFramework(source, externalType, framework string) {
	source = strings.ToLower(strings.TrimSpace(source))
	externalType = strings.ToLower(strings.TrimSpace(externalType))
	framework = strings.ToLower(strings.TrimSpace(framework))
	if source == "" || externalType == "" || framework == "" {
		return
	}
	if migrationFrameworks[source] == nil {
		migrationFrameworks[source] = map[string]string{}
	}
	migrationFrameworks[source][externalType] = framework
}

func lookupMigrationFramework(source, externalType string) string {
	normalized := strings.ToLower(strings.TrimSpace(externalType))
	if framework := migrationFrameworks[strings.ToLower(strings.TrimSpace(source))][normalized]; framework != "" {
		return framework
	}
	return externalType
}
