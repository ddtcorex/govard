package tests

import (
	"reflect"
	"testing"

	"govard/internal/frameworks/magento2"
)

func TestMergeComposerMapKeys(t *testing.T) {
	tests := []struct {
		name     string
		current  map[string]interface{}
		target   map[string]interface{}
		key      string
		expected map[string]interface{}
	}{
		{
			name: "add new object key",
			current: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/package": "1.0.0",
				},
			},
			target: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/core": "2.4.8",
				},
			},
			key: "require",
			expected: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/package": "1.0.0",
					"sample/core":    "2.4.8",
				},
			},
		},
		{
			name: "override existing key in object",
			current: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/core":    "2.4.7",
					"sample/package": "1.0.0",
				},
			},
			target: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/core": "2.4.8",
				},
			},
			key: "require",
			expected: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/package": "1.0.0",
					"sample/core":    "2.4.8",
				},
			},
		},
		{
			name: "scalar values replacement",
			current: map[string]interface{}{
				"minimum-stability": "dev",
			},
			target: map[string]interface{}{
				"minimum-stability": "stable",
			},
			key: "minimum-stability",
			expected: map[string]interface{}{
				"minimum-stability": "stable",
			},
		},
		{
			name:    "key missing in current",
			current: map[string]interface{}{},
			target: map[string]interface{}{
				"require-dev": map[string]interface{}{
					"test/runner": "9.0",
				},
			},
			key: "require-dev",
			expected: map[string]interface{}{
				"require-dev": map[string]interface{}{
					"test/runner": "9.0",
				},
			},
		},
		{
			name: "key missing in target",
			current: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/package": "1.0",
				},
			},
			target: map[string]interface{}{},
			key:    "require",
			expected: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/package": "1.0",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			magento2.MergeComposerMapKeysForTest(tt.current, tt.target, tt.key)
			if !reflect.DeepEqual(tt.current, tt.expected) {
				t.Errorf("MergeComposerMapKeysForTest() mismatch.\nGot:  %v\nWant: %v", tt.current, tt.expected)
			}
		})
	}
}
