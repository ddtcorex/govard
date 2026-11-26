package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"govard/internal/engine"
)

func TestDirectDBImportDropFailsOnMissingFileBeforeReset(t *testing.T) {
	config := engine.Config{ProjectName: "sample-project", Framework: "magento2"}
	options := dbCommandOptions{
		Environment: "local",
		File:        filepath.Join(t.TempDir(), "missing.sql"),
		Drop:        true,
		AssumeYes:   true,
	}

	err := runDirectDBImport(&cobra.Command{}, config, options)
	if err == nil {
		t.Fatal("runDirectDBImport succeeded with a missing import file, want an error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error %q does not wrap os.ErrNotExist; the reset must not run before the file is opened", err)
	}
	if !strings.Contains(err.Error(), "open import file") {
		t.Fatalf("error %q does not name the import file", err)
	}
}
