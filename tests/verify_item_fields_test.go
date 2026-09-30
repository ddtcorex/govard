package tests

import (
	"reflect"
	"testing"

	"govard/internal/verify"
)

func TestNoDeadItemFields(t *testing.T) {
	if _, ok := reflect.TypeOf(verify.Item{}).FieldByName("Timeout"); ok {
		t.Error("Item.Timeout is read by nothing and must be deleted")
	}
	if _, ok := reflect.TypeOf(verify.RunItem{}).FieldByName("Retries"); ok {
		t.Error("RunItem.Retries is always 0 and must be deleted")
	}
	if _, ok := reflect.TypeOf(verify.Evidence{}).FieldByName("Retries"); ok {
		t.Error("Evidence.Retries feeds only RunItem.Retries and must be deleted")
	}
	if _, ok := reflect.TypeOf(verify.Item{}).FieldByName("Precond"); ok {
		t.Error("Item.Precond was renamed Requires")
	}
	if _, ok := reflect.TypeOf(verify.Item{}).FieldByName("Requires"); !ok {
		t.Error("Item.Requires (documentation only) must exist")
	}
}

func TestGuardRemoteProbeLabel(t *testing.T) {
	if verify.GuardRemoteProbe != "REMOTE-PROBE" {
		t.Fatalf("GuardRemoteProbe = %q, want REMOTE-PROBE", verify.GuardRemoteProbe)
	}
	for _, it := range verify.Registry {
		if it.Guard == "READ-ONLY-REMOTE" {
			t.Errorf("%s still carries the retired READ-ONLY-REMOTE label", it.ID)
		}
	}
}
