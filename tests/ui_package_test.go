package tests

import (
	"testing"

	"govard/internal/ui"
)

func TestUIPkgPrintBrandDoesNotPanic(t *testing.T) {
	ui.PrintBrand("1.3.0")
}

func TestUIPkgPrintHelpersDoNotPanic(t *testing.T) {
	ui.PrintSuccess("ok")
	ui.PrintError("error")
	ui.PrintInfo("info")
}
