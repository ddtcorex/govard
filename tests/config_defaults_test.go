package tests

import (
	"govard/internal/engine"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNormalizeConfigChownDirListDefaults(t *testing.T) {
	tests := []struct {
		name      string
		framework string
		want      []string
	}{
		{
			name:      "Magento 2 defaults",
			framework: "magento2",
			want:      []string{"/bash_history", "/var/www/html", "/home/www-data/.cache/composer"},
		},
		{
			name:      "Mage-OS defaults",
			framework: "mageos",
			want:      []string{"/bash_history", "/var/www/html", "/home/www-data/.cache/composer"},
		},
		{
			name:      "Generic framework defaults",
			framework: "laravel",
			want:      []string{"/bash_history"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := engine.Config{
				Framework: tt.framework,
			}
			engine.NormalizeConfig(&config, "")
			if len(config.Stack.ChownDirList) != len(tt.want) {
				t.Fatalf("expected %d items, got %d: %v", len(tt.want), len(config.Stack.ChownDirList), config.Stack.ChownDirList)
			}
			for i, v := range tt.want {
				if config.Stack.ChownDirList[i] != v {
					t.Errorf("expected item %d to be %q, got %q", i, v, config.Stack.ChownDirList[i])
				}
			}
		})
	}
}

func TestPrepareConfigForWriteOmitsDefaultChownDirList(t *testing.T) {
	config := engine.Config{
		Framework: "magento2",
		Stack: engine.Stack{
			ChownDirList: []string{"/bash_history", "/var/www/html", "/home/www-data/.cache/composer"},
		},
	}

	writable := engine.PrepareConfigForWrite(config)
	if writable.Stack.ChownDirList != nil {
		t.Fatalf("expected ChownDirList to be cleared when matching defaults, got %v", writable.Stack.ChownDirList)
	}

	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatalf("marshal writable config: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "chown_dir_list:") {
		t.Fatalf("expected serialized config to omit chown_dir_list, got:\n%s", content)
	}
}

func TestPrepareConfigForWriteKeepsCustomChownDirList(t *testing.T) {
	customList := []string{"/bash_history", "/var/www/html/pub/static"}
	config := engine.Config{
		Framework: "magento2",
		Stack: engine.Stack{
			ChownDirList: customList,
		},
	}

	writable := engine.PrepareConfigForWrite(config)
	if len(writable.Stack.ChownDirList) != len(customList) {
		t.Fatalf("expected custom ChownDirList to persist, got %v", writable.Stack.ChownDirList)
	}

	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatalf("marshal writable config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "chown_dir_list:") {
		t.Fatalf("expected serialized config to include custom chown_dir_list, got:\n%s", content)
	}
	if !strings.Contains(content, "/var/www/html/pub/static") {
		t.Fatalf("expected custom path in serialized config, got:\n%s", content)
	}
}

func TestPrepareConfigForWriteOmitsFalseFeatures(t *testing.T) {
	config := engine.Config{
		Framework: "magento2",
		Stack: engine.Stack{
			Features: engine.Features{
				Varnish: false,
				Xdebug:  false,
			},
		},
	}

	writable := engine.PrepareConfigForWrite(config)
	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatalf("marshal writable config: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "varnish:") {
		t.Fatalf("expected serialized config to omit varnish: false, got:\n%s", content)
	}
	if strings.Contains(content, "xdebug:") {
		t.Fatalf("expected serialized config to omit xdebug: false, got:\n%s", content)
	}
}

func TestPrepareConfigForWriteKeepsTrueFeatures(t *testing.T) {
	config := engine.Config{
		Framework: "magento2",
		Stack: engine.Stack{
			Features: engine.Features{
				Varnish: true,
				Xdebug:  true,
			},
		},
	}

	writable := engine.PrepareConfigForWrite(config)
	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatalf("marshal writable config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "varnish: true") {
		t.Fatalf("expected serialized config to include varnish: true, got:\n%s", content)
	}
	if !strings.Contains(content, "xdebug: true") {
		t.Fatalf("expected serialized config to include xdebug: true, got:\n%s", content)
	}
}
