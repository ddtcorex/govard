package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMagento2VarnishTemplateTracksOfficialMagentoDefaults(t *testing.T) {
	projectRoot := testProjectRoot(t)
	vclPath := filepath.Join(projectRoot, "internal", "frameworks", "magento2", "blueprint", "varnish", "default.vcl")

	content, err := os.ReadFile(vclPath)
	if err != nil {
		t.Fatalf("read %s: %v", vclPath, err)
	}
	vcl := string(content)

	for _, expected := range []string{
		`set req.url = std.querysort(req.url);`,
		`gad_source|gbraid|wbraid|_gl|dclid|gclsrc|srsltid|msclkid|gclid|cx|_kx`,
		`if (obj.ttl + 300s > 0s) {`,
		`return (synth(400, "X-Magento-Tags-Pattern or X-Pool header required"));`,
	} {
		if !strings.Contains(vcl, expected) {
			t.Fatalf("expected Magento 2 Varnish template to contain %q, got:\n%s", expected, vcl)
		}
	}

	if strings.Contains(vcl, `unset resp.http.Content-Security-Policy;`) {
		t.Fatalf("expected Magento 2 Varnish template to preserve Content-Security-Policy like the official template, got:\n%s", vcl)
	}
}
