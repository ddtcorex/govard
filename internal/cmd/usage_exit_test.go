package cmd

import (
	"errors"
	"testing"

	"govard/internal/cli"
)

func TestArgumentErrorsExitAsUsage(t *testing.T) {
	err := asUsageIfArgumentError(errors.New("accepts 1 arg(s), received 0"))
	if got := cli.Code(err); got != cli.CodeUsage {
		t.Fatalf("exit code = %d, want %d", got, cli.CodeUsage)
	}
}

func TestUnknownCommandExitsAsUsage(t *testing.T) {
	err := asUsageIfArgumentError(errors.New(`unknown command "bogus" for "govard"`))
	if got := cli.Code(err); got != cli.CodeUsage {
		t.Fatalf("exit code = %d, want %d", got, cli.CodeUsage)
	}
}

func TestExecutionErrorsStayExecution(t *testing.T) {
	err := asUsageIfArgumentError(errors.New("failed to load config"))
	if got := cli.Code(err); got != cli.CodeError {
		t.Fatalf("exit code = %d, want %d", got, cli.CodeError)
	}
}
