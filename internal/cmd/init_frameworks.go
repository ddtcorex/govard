package cmd

import (
	"sort"

	"govard/internal/frameworks"
)

// FrameworkSelectionOption is the presentation data used by the init picker.
// It comes from the registry so the CLI and desktop cannot drift apart.
type FrameworkSelectionOption struct {
	Name        string
	DisplayName string
}

func frameworkSelectionOptions() []FrameworkSelectionOption {
	definitions := frameworks.All()
	options := make([]FrameworkSelectionOption, 0, len(definitions))
	for _, definition := range definitions {
		options = append(options, FrameworkSelectionOption{
			Name:        definition.Name,
			DisplayName: definition.DisplayName,
		})
	}
	sort.Slice(options, func(i, j int) bool {
		return options[i].DisplayName < options[j].DisplayName
	})
	return options
}

// InitFrameworkOptionsForTest exposes the picker data for a registry-parity
// test without coupling that test to interactive terminal input.
func InitFrameworkOptionsForTest() []FrameworkSelectionOption {
	return frameworkSelectionOptions()
}
