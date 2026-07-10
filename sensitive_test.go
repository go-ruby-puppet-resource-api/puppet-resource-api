// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import (
	"fmt"
	"testing"
)

func TestSensitiveWrapAndRedact(t *testing.T) {
	s := NewSensitive("hunter2")
	if s.Unwrap() != "hunter2" {
		t.Errorf("Unwrap = %v", s.Unwrap())
	}
	if s.String() != RedactedString {
		t.Errorf("String = %q", s.String())
	}
	if s.GoString() != RedactedString {
		t.Errorf("GoString = %q", s.GoString())
	}
	// fmt must not leak the value through either verb.
	if got := fmt.Sprintf("%v/%#v", s, s); got != "[redacted]/[redacted]" {
		t.Errorf("fmt leaked: %q", got)
	}
	// Double-wrapping is a no-op.
	if NewSensitive(s) != s {
		t.Error("double wrap should return the same pointer")
	}
}

func TestValidateWrapsSensitive(t *testing.T) {
	ty := mustCompile(t, sensitiveDef())
	got, err := ty.Validate(Resource{"name": "db", "password": "s3cr3t"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	sv, ok := got["password"].(*Sensitive)
	if !ok {
		t.Fatalf("password not wrapped: %T", got["password"])
	}
	if sv.Unwrap() != "s3cr3t" {
		t.Errorf("wrapped value = %v", sv.Unwrap())
	}
	// name (non-sensitive) stays plain.
	if _, wrapped := got["name"].(*Sensitive); wrapped {
		t.Error("name should not be wrapped")
	}
}

func TestValidateSensitiveTypeCheckedUnwrapped(t *testing.T) {
	ty := mustCompile(t, sensitiveDef())
	// A pre-wrapped value is unwrapped for the type check and re-wrapped.
	got, err := ty.Validate(Resource{"name": "db", "password": NewSensitive("pw")})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got["password"].(*Sensitive).Unwrap() != "pw" {
		t.Errorf("got %v", got["password"])
	}
	// A wrapped value of the wrong type still fails the type check.
	if _, err := ty.Validate(Resource{"name": "db", "password": NewSensitive(42)}); err == nil {
		t.Error("expected type error for wrapped non-string")
	}
}

func TestRedact(t *testing.T) {
	ty := mustCompile(t, sensitiveDef())
	r := Resource{
		"name":     "db",
		"password": NewSensitive("s3cr3t"),
		"ensure":   "present",
	}
	red := ty.Redact(r)
	if red["password"] != RedactedString {
		t.Errorf("password not redacted: %v", red["password"])
	}
	if red["name"] != "db" || red["ensure"] != "present" {
		t.Errorf("non-sensitive changed: %v", red)
	}
	// A declared-sensitive attribute stored unwrapped is still redacted.
	red2 := ty.Redact(Resource{"name": "db", "password": "raw"})
	if red2["password"] != RedactedString {
		t.Errorf("declared-sensitive not redacted: %v", red2)
	}
	// The source resource is untouched.
	if r["password"].(*Sensitive).Unwrap() != "s3cr3t" {
		t.Error("Redact mutated source")
	}
}

func TestSensitiveEquality(t *testing.T) {
	// Two sensitive wrappers around the same content compare equal; different
	// content differs, so Apply can still detect a change without leaking.
	if !equalAny(NewSensitive("x"), NewSensitive("x")) {
		t.Error("equal content should be equal")
	}
	if equalAny(NewSensitive("x"), NewSensitive("y")) {
		t.Error("different content should differ")
	}
	// A wrapped value equals its unwrapped counterpart.
	if !equalAny(NewSensitive("x"), "x") {
		t.Error("wrapped should equal unwrapped")
	}
}

func TestApplySensitiveChangeDetected(t *testing.T) {
	ty := mustCompile(t, sensitiveDef())
	mem := newMem()
	// Current password differs from desired -> update, even though both are
	// sensitive.
	mem.store["db"] = Resource{"name": "db", "password": NewSensitive("old"), "ensure": "present"}
	p := SimpleProvider{Crud: mem}
	sum, err := ty.Apply(NewContext(ty, nil), p, []Resource{{"name": "db", "password": "new"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(sum.Updated) != 1 {
		t.Fatalf("expected update, got %+v", sum)
	}
	// Identical sensitive value -> unchanged.
	mem.store["db"] = Resource{"name": "db", "password": NewSensitive("same"), "ensure": "present"}
	sum, err = ty.Apply(NewContext(ty, nil), p, []Resource{{"name": "db", "password": "same"}})
	if err != nil {
		t.Fatalf("Apply2: %v", err)
	}
	if len(sum.Unchanged) != 1 {
		t.Fatalf("expected unchanged, got %+v", sum)
	}
}
