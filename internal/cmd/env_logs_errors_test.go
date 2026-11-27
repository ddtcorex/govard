package cmd

import (
	"strings"
	"testing"
)

func TestErrorLogsCommandQuotesArguments(t *testing.T) {
	command := buildErrorLogsCommand("/srv/sample project", "sample-project", "/home/u/.govard/compose/sample-project.yml", []string{"logs", "php; touch /tmp/x"})

	if !strings.Contains(command, `'php; touch /tmp/x'`) {
		t.Fatalf("command %q does not single-quote the argument, so the shell would split and run it", command)
	}
	if !strings.Contains(command, `'/srv/sample project'`) || !strings.Contains(command, `'/home/u/.govard/compose/sample-project.yml'`) {
		t.Fatalf("command %q leaves a path with a space unquoted", command)
	}
	if !strings.HasSuffix(command, "| grep -iE 'error|critical|fail|exception'") {
		t.Fatalf("command %q lost the error filter", command)
	}
}

func TestErrorLogsCommandDetectsFollowExactly(t *testing.T) {
	withoutFollow := buildErrorLogsCommand("/srv/p", "p", "/c/p.yml", []string{"logs", "--no-log-prefix"})
	if !strings.Contains(withoutFollow, "-f --tail=100") {
		t.Fatalf("command %q does not add follow for a flag that merely contains -f", withoutFollow)
	}

	for _, followArgs := range [][]string{
		{"logs", "--follow"},
		{"logs", "-f"},
		{"logs", "--follow=true"},
	} {
		command := buildErrorLogsCommand("/srv/p", "p", "/c/p.yml", followArgs)
		if strings.Contains(command, "--tail=100") {
			t.Fatalf("command %q for %v adds a second follow", command, followArgs)
		}
	}
}
