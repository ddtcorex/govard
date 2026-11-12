package tests

import (
	"errors"
	"fmt"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

// TestHumanErrorMessageDropsDetailThatWasAlreadyShown pins the terminal form of a
// compose failure. Compose prints its own diagnosis on stderr, so repeating it in
// the final ERROR line shows the person the same sentence twice; the machine
// envelope keeps the detail because nobody parsed the stream above it.
func TestHumanErrorMessageDropsDetailThatWasAlreadyShown(t *testing.T) {
	base := errors.New("exit status 1")
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "streamed detail is dropped under wrapping",
			err:  fmt.Errorf("failed to down containers: %w", &engine.ComposeError{Err: base, Detail: "no such service: web", Streamed: true}),
			want: "failed to down containers: exit status 1",
		},
		{
			name: "unstreamed detail is the only copy and stays",
			err:  fmt.Errorf("failed to down containers: %w", &engine.ComposeError{Err: base, Detail: "no such service: web"}),
			want: "failed to down containers: exit status 1: no such service: web",
		},
		{
			name: "a plain error is untouched",
			err:  errors.New("plain"),
			want: "plain",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cmd.HumanErrorMessageForTest(tc.err); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
