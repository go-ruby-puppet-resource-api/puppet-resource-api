// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import "testing"

func mustCompile(t *testing.T, d Definition) *Type {
	t.Helper()
	ty, err := Compile(d)
	if err != nil {
		t.Fatalf("Compile(%s): %v", d.Name, err)
	}
	return ty
}

func TestValidateGood(t *testing.T) {
	ty := mustCompile(t, personDef())
	got, err := ty.Validate(Resource{
		"name":  "alice",
		"email": "Alice@Example.COM",
		"role":  "admin",
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got["age"] != 30 {
		t.Errorf("age default not applied: %v", got["age"])
	}
	if got["email"] != "alice@example.com" {
		t.Errorf("munge not applied: %v", got["email"])
	}
	if got["role"] != "admin" {
		t.Errorf("role override lost: %v", got["role"])
	}
	if got["ensure"] != "present" {
		t.Errorf("ensure default: %v", got["ensure"])
	}
}

func TestValidateTitleFillsNamevar(t *testing.T) {
	ty := mustCompile(t, personDef())
	got, err := ty.Validate(Resource{TitleKey: "bob"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got["name"] != "bob" {
		t.Errorf("name from title = %v", got["name"])
	}
}

func TestValidateNamevarNotOverriddenByTitle(t *testing.T) {
	ty := mustCompile(t, personDef())
	got, err := ty.Validate(Resource{TitleKey: "bob", "name": "carol"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got["name"] != "carol" {
		t.Errorf("name should stay carol, got %v", got["name"])
	}
}

func TestValidateDefaultNotOverridden(t *testing.T) {
	ty := mustCompile(t, personDef())
	got, err := ty.Validate(Resource{"name": "d", "age": 40})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got["age"] != 40 {
		t.Errorf("age should stay 40, got %v", got["age"])
	}
}

func TestValidateErrors(t *testing.T) {
	ty := mustCompile(t, personDef())
	cases := []struct {
		name string
		in   Resource
		attr string
		want string
	}{
		{"unknown attr", Resource{"name": "a", "bogus": 1}, "bogus", "unknown attribute"},
		{"read only", Resource{"name": "a", "ssn": "123"}, "ssn", "read-only"},
		{"missing namevar", Resource{"age": 20}, "name", "namevar is required"},
		{"munge error", Resource{"name": "a", "email": 123}, "email", "munge failed"},
		{"validate hook", Resource{"name": "a", "email": "noatsign"}, "email", "validation failed"},
		{"type mismatch", Resource{"name": "a", "age": 200}, "age", "does not match type"},
		{"empty namevar", Resource{"name": ""}, "name", "does not match type"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ty.Validate(c.in)
			if err == nil {
				t.Fatalf("expected error %q", c.want)
			}
			ve, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("want *ValidationError, got %T", err)
			}
			if ve.Attribute != c.attr {
				t.Errorf("attribute = %q want %q", ve.Attribute, c.attr)
			}
			if !contains(ve.Msg, c.want) {
				t.Errorf("msg %q missing %q", ve.Msg, c.want)
			}
		})
	}
}

func TestValidateDoesNotMutateInput(t *testing.T) {
	ty := mustCompile(t, personDef())
	in := Resource{"name": "a"}
	if _, err := ty.Validate(in); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, ok := in["age"]; ok {
		t.Error("input was mutated")
	}
}

func TestTitle(t *testing.T) {
	person := mustCompile(t, personDef())
	link := mustCompile(t, linkDef())

	if got, err := person.Title(Resource{TitleKey: "x"}); err != nil || got != "x" {
		t.Errorf("explicit title: %q %v", got, err)
	}
	if _, err := person.Title(Resource{TitleKey: 5}); err == nil {
		t.Error("non-string title should error")
	}
	if got, err := person.Title(Resource{"name": "z"}); err != nil || got != "z" {
		t.Errorf("namevar title: %q %v", got, err)
	}
	if _, err := person.Title(Resource{}); err == nil {
		t.Error("no title/namevar should error")
	}
	if _, err := person.Title(Resource{"name": 5}); err == nil {
		t.Error("non-string namevar should error")
	}
	if _, err := link.Title(Resource{"source": "a", "target": "b"}); err == nil {
		t.Error("multi-namevar without title should error")
	}
}

func TestTitlePatternDecompose(t *testing.T) {
	link := mustCompile(t, linkDef())
	got, err := link.Validate(Resource{TitleKey: "a:b"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got["source"] != "a" || got["target"] != "b" {
		t.Errorf("decompose = %v", got)
	}
}

func TestTitlePatternKeepsPreset(t *testing.T) {
	link := mustCompile(t, linkDef())
	got, err := link.Validate(Resource{TitleKey: "a:b", "source": "x"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got["source"] != "x" {
		t.Errorf("preset source overwritten: %v", got["source"])
	}
	if got["target"] != "b" {
		t.Errorf("target = %v", got["target"])
	}
}

func TestTitlePatternNoMatch(t *testing.T) {
	link := mustCompile(t, linkDef())
	// A title without a colon matches no pattern; both namevars must then be
	// supplied explicitly or validation fails.
	_, err := link.Validate(Resource{TitleKey: "nocolon"})
	if err == nil {
		t.Fatal("expected missing namevar error")
	}
}
