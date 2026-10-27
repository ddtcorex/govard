package tests

import (
	"reflect"
	"testing"

	"govard/internal/cmd"
)

func TestBootstrapAdminLines(t *testing.T) {
	tests := []struct {
		name      string
		framework string
		domain    string
		applied   bool
		want      []string
	}{
		{
			name:      "magento2 fresh install",
			framework: "magento2",
			domain:    "sample.test",
			applied:   true,
			want: []string{
				"Admin:     https://sample.test/admin",
				"Login:     admin / AdminGovard123$",
			},
		},
		{
			name:      "magento2 clone keeps the source credential",
			framework: "magento2",
			domain:    "sample.test",
			applied:   false,
			want:      []string{"Admin:     https://sample.test/admin"},
		},
		{
			name:      "wordpress uses its own admin route",
			framework: "wordpress",
			domain:    "sample.test",
			applied:   true,
			want: []string{
				"Admin:     https://sample.test/wp-admin",
				"Login:     admin / AdminGovard123$",
			},
		},
		{
			name:      "laravel has no stock admin route",
			framework: "laravel",
			domain:    "sample.test",
			applied:   true,
			want:      nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cmd.BootstrapAdminLinesForTest(tc.framework, tc.domain, tc.applied); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("BootstrapAdminLines(%q, applied=%v) = %#v, want %#v", tc.framework, tc.applied, got, tc.want)
			}
		})
	}
}
