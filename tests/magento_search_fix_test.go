package tests

import (
	"strings"
	"testing"

	"govard/internal/frameworks/magento2"
)

func TestBuildMagentoSearchHostFixSQL(t *testing.T) {
	tests := []struct {
		name         string
		host         string
		searchEngine string
		expected     []string
	}{
		{
			name:         "Default engine and host",
			host:         "elasticsearch",
			searchEngine: "elasticsearch7",
			expected: []string{
				"SET @table_name = (SELECT TABLE_NAME FROM information_schema.TABLES",
				"IF(@table_name IS NOT NULL",
				"value = \"elasticsearch\"",
				"value = \"9200\"",
				"value = \"0\"",
				"value = \"elasticsearch:9200\"",
				"catalog/search/engine'', ''elasticsearch7''",
			},
		},
		{
			name:         "OpenSearch config",
			host:         "opensearch",
			searchEngine: "opensearch",
			expected: []string{
				"value = \"opensearch\"",
				"value = \"opensearch:9200\"",
				"catalog/search/engine'', ''opensearch''",
			},
		},
		{
			name:         "Empty engine",
			host:         "elasticsearch",
			searchEngine: "",
			expected: []string{
				"value = \"elasticsearch\"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql := magento2.BuildMagentoSearchHostFixSQL(tt.host, tt.searchEngine)

			for _, exp := range tt.expected {
				if !strings.Contains(sql, exp) {
					t.Errorf("expected SQL to contain %q, got: %s", exp, sql)
				}
			}

			if tt.searchEngine == "" && strings.Contains(sql, "catalog/search/engine") {
				t.Errorf("did not expect catalog/search/engine update when searchEngine is empty, got: %s", sql)
			}
		})
	}
}
