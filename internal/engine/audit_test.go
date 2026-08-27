package engine

import (
	"runtime"
	"testing"
)

func TestLintJobsAuto(t *testing.T) {
	jobs := AuditJobs()
	if jobs < 2 || jobs > 8 {
		t.Fatalf("jobs must be 2-8, got %d", jobs)
	}
	if jobs != min(runtime.NumCPU(), 4) && runtime.NumCPU() > 4 {
		t.Fatal("jobs must be min(nproc,4)")
	}
}

func TestDiffScope(t *testing.T) {
	if AuditScope(true, "HEAD") != "diff" {
		t.Fatal("diff scope")
	}
	if AuditScope(false, "") != "project" {
		t.Fatal("project scope")
	}
	if AuditScope(true, "") != "project" {
		t.Fatal("empty base must fallback to project")
	}
	if AuditScope(true, "origin/master") != "project" && AuditScope(true, "origin/master") != "diff" {
		t.Fatal("origin/master must be diff or project (CI may not fetch origin)")
	}
}
