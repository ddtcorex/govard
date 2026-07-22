package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestResolveToolExecutionForDjangoSkipsWWWData(t *testing.T) {
	config := engine.Config{ProjectName: "myproj", Framework: "django"}
	container, _, user := cmd.ResolveToolExecutionForTest(config, "python")

	if !strings.Contains(container, "-web-") {
		t.Errorf("expected container name to reference the web service, got %s", container)
	}
	if user == "www-data" {
		t.Error("did not expect www-data user resolution for a django (python-runtime) framework")
	}
}
