package tests

import (
	"reflect"
	"sort"
	"testing"

	"govard/internal/cmd"
	"govard/internal/frameworks"
)

// This catches the old hard-coded init picker drifting from the registry (it
// previously omitted Django and would have omitted newly registered Custom).
func TestInitFrameworkOptionsMatchRegistry(t *testing.T) {
	got := cmd.InitFrameworkOptionsForTest()

	want := make([]cmd.FrameworkSelectionOption, 0)
	for _, definition := range frameworks.All() {
		want = append(want, cmd.FrameworkSelectionOption{
			Name:        definition.Name,
			DisplayName: definition.DisplayName,
		})
	}
	sort.Slice(want, func(i, j int) bool { return want[i].DisplayName < want[j].DisplayName })

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("init framework options = %#v, want registry-derived %#v", got, want)
	}
}
