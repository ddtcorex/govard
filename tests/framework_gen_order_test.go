package tests

import (
	"reflect"
	"testing"

	"govard/internal/frameworks/gen/generator"
)

func TestOrderByPriorityDefaultsToAlphabetical(t *testing.T) {
	got := generator.OrderByPriority([]string{"django", "cakephp", "laravel"}, map[string]int{})
	want := []string{"cakephp", "django", "laravel"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("OrderByPriority() = %v, want %v", got, want)
	}
}

func TestOrderByPriorityAppliesOverrides(t *testing.T) {
	got := generator.OrderByPriority(
		[]string{"cakephp", "emdash", "nextjs"},
		map[string]int{"emdash": -1},
	)
	want := []string{"emdash", "cakephp", "nextjs"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("OrderByPriority() = %v, want %v", got, want)
	}
}

func TestOrderByPriorityDoesNotMutateInput(t *testing.T) {
	input := []string{"django", "cakephp"}
	_ = generator.OrderByPriority(input, map[string]int{})
	want := []string{"django", "cakephp"}
	if !reflect.DeepEqual(input, want) {
		t.Errorf("OrderByPriority() mutated its input slice: got %v, want %v", input, want)
	}
}

func TestPriorityOverridesPreservesEmdashBeforeNextjs(t *testing.T) {
	got := generator.OrderByPriority([]string{"nextjs", "emdash"}, generator.PriorityOverrides)
	want := []string{"emdash", "nextjs"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("OrderByPriority() with PriorityOverrides = %v, want %v", got, want)
	}
}
