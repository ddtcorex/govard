package tests

import (
	"testing"

	wordpress "govard/internal/frameworks/wordpress"
)

func TestParseWPCliVersion(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"WP-CLI 2.8.1", "2.8.1"},
		{"WP-CLI 2.10.0", "2.10.0"},
		{"WP-CLI 1.4.0", "1.4.0"},
		{"", ""},
		{"WP-CLI x.y.z", ""},
		{"no version here", ""},
		{"php warning noise\nWP-CLI 2.8.1\n", "2.8.1"},
	}
	for _, c := range cases {
		if got := wordpress.ParseWPCliVersionForTest(c.raw); got != c.want {
			t.Errorf("ParseWPCliVersionForTest(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestWPCliVersionMatches(t *testing.T) {
	cases := []struct {
		raw   string
		want  string
		match bool
	}{
		{"WP-CLI 2.10.0", "2.10.0", true},
		{"WP-CLI 2.8.1", "2.10.0", false},
		{"WP-CLI 1.4.0", "2.10.0", false},
		{"", "2.10.0", false},
		{"WP-CLI 2.10.0", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := wordpress.WPCliVersionMatchesForTest(c.raw, c.want); got != c.match {
			t.Errorf("WPCliVersionMatchesForTest(%q, %q) = %v, want %v", c.raw, c.want, got, c.match)
		}
	}
}

func TestRecommendedWPCliVersion(t *testing.T) {
	cases := []struct {
		major int
		want  string
	}{
		{4, "2.4.0"},
		{5, "2.8.1"},
		{6, "2.10.0"},
		{7, ""},
		{0, ""},
	}
	for _, c := range cases {
		if got := wordpress.RecommendedWPCliVersionForTest(c.major); got != c.want {
			t.Errorf("RecommendedWPCliVersionForTest(%d) = %q, want %q", c.major, got, c.want)
		}
	}
}
