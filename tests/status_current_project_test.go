package tests

import (
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func statusRows(data [][]string) []string {
	var rows []string
	for _, row := range data[1:] {
		rows = append(rows, row[0]+"/"+row[1])
	}
	return rows
}

func TestStatusListsStoppedCurrentProjectFirst(t *testing.T) {
	data := cmd.StatusTableDataForTest("sample-project", []string{"other"}, nil)
	got := statusRows(data)
	if len(got) != 2 || got[0] != "sample-project/Down" || got[1] != "other/Running" {
		t.Fatalf("rows = %v, want the stopped current project first, then running ones", got)
	}
}

func TestStatusCurrentRunningNotDuplicated(t *testing.T) {
	got := statusRows(cmd.StatusTableDataForTest("sample-project", []string{"other", "sample-project"}, nil))
	if len(got) != 2 || got[0] != "sample-project/Running" || got[1] != "other/Running" {
		t.Fatalf("rows = %v, want the current project once, first, Running", got)
	}
}

func TestStatusOutsideProjectNothingRunning(t *testing.T) {
	if data := cmd.StatusTableDataForTest("", nil, nil); data != nil {
		t.Fatalf("data = %v, want nil so the caller prints the empty message", data)
	}
}

func TestStatusCurrentDownNothingElseRunning(t *testing.T) {
	got := statusRows(cmd.StatusTableDataForTest("sample-project", nil, []engine.ProjectRegistryEntry{{ProjectName: "sample-project", Domain: "sample.test"}}))
	if len(got) != 1 || got[0] != "sample-project/Down" {
		t.Fatalf("rows = %v, want one Down row", got)
	}
}
