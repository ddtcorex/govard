package tests

import (
	"reflect"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestProjectListPutsRunningProjectsFirstWithStatus(t *testing.T) {
	entries := []engine.ProjectRegistryEntry{
		{ProjectName: "alpha", Framework: "magento2"},
		{ProjectName: "beta"},
		{ProjectName: "gamma", Framework: "laravel"},
	}
	rows := cmd.OrderProjectListRowsForTest(entries, map[string]bool{"gamma": true})
	var got [][2]string
	for _, r := range rows {
		got = append(got, [2]string{r[0], r[1]})
	}
	want := [][2]string{{"gamma", "running"}, {"alpha", "stopped"}, {"beta", "stopped"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if rows[2][2] != "unknown" {
		t.Fatalf("empty framework should read unknown, got %q", rows[2][2])
	}
}

func TestProjectListKeepsRegistryOrderWhenRuntimeIsUnreachable(t *testing.T) {
	entries := []engine.ProjectRegistryEntry{{ProjectName: "b"}, {ProjectName: "a"}}
	rows := cmd.OrderProjectListRowsForTest(entries, nil)
	if rows[0][0] != "b" || rows[0][1] != "unknown" || rows[1][1] != "unknown" {
		t.Fatalf("unexpected rows %v", rows)
	}
}
