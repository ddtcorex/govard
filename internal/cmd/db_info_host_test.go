package cmd

import "testing"

func TestDBInfoHostLineNeverPrintsBlankHost(t *testing.T) {
	cases := []struct{ host, fallback, want string }{
		{"db.example", "localhost", "db.example"},
		{"", "localhost (inside container)", "localhost (inside container)"},
		{"  ", "localhost", "localhost"},
	}
	for _, c := range cases {
		if got := dbInfoHost(c.host, c.fallback); got != c.want {
			t.Fatalf("dbInfoHost(%q) = %q, want %q", c.host, got, c.want)
		}
	}
}
