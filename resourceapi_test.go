// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import (
	"testing"
)

func TestCompileSuccess(t *testing.T) {
	ty, err := Compile(personDef())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if ty.Name() != "person" {
		t.Errorf("Name = %q", ty.Name())
	}
	if got := ty.Definition().Desc; got != "a person" {
		t.Errorf("Definition().Desc = %q", got)
	}
	names := ty.AttributeNames()
	if len(names) != 8 || names[0] != "age" {
		t.Errorf("AttributeNames = %v", names)
	}
	if nv := ty.Namevars(); len(nv) != 1 || nv[0] != "name" {
		t.Errorf("Namevars = %v", nv)
	}
	if ty.HasFeature("canonicalize") {
		t.Error("unexpected feature")
	}
}

func TestCompileErrors(t *testing.T) {
	base := func() Definition { return personDef() }
	cases := []struct {
		name string
		mut  func(d *Definition)
		want string
	}{
		{"bad name", func(d *Definition) { d.Name = "Bad-Name" }, "name must match"},
		{"empty name", func(d *Definition) { d.Name = "" }, "name must match"},
		{"no attributes", func(d *Definition) { d.Attributes = nil }, "at least one attribute"},
		{"empty type", func(d *Definition) {
			d.Attributes = map[string]Attribute{"name": {Behaviour: Namevar}}
		}, "type is required"},
		{"bad behaviour", func(d *Definition) {
			d.Attributes = map[string]Attribute{"name": {Type: "String", Behaviour: "weird"}}
		}, "unknown behaviour"},
		{"bad type expr", func(d *Definition) {
			d.Attributes = map[string]Attribute{"name": {Type: "Nope[[", Behaviour: Namevar}}
		}, "invalid type"},
		{"default not instance", func(d *Definition) {
			d.Attributes = map[string]Attribute{
				"name": {Type: "String", Behaviour: Namevar},
				"age":  {Type: "String", HasDefault: true, Default: 5},
			}
		}, "default is not a"},
		{"no namevar", func(d *Definition) {
			d.Attributes = map[string]Attribute{"x": {Type: "String"}}
		}, "at least one namevar"},
		{"empty feature", func(d *Definition) { d.Features = []string{"ok", ""} }, "is empty"},
		{"bad autorelation", func(d *Definition) {
			d.Autorequire = map[string]string{"file": "nope"}
		}, "references unknown attribute"},
		{"bad title regexp", func(d *Definition) {
			d.TitlePatterns = []TitlePattern{{Pattern: "("}}
		}, "title_pattern at index 0"},
		{"title capture not attr", func(d *Definition) {
			d.TitlePatterns = []TitlePattern{{Pattern: `(?P<bogus>.+)`}}
		}, "is not an attribute"},
		{"title no named captures", func(d *Definition) {
			d.TitlePatterns = []TitlePattern{{Pattern: `^nope$`}}
		}, "no named captures"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := base()
			c.mut(&d)
			_, err := Compile(d)
			if err == nil {
				t.Fatalf("expected error containing %q", c.want)
			}
			de, ok := err.(*DefinitionError)
			if !ok {
				t.Fatalf("want *DefinitionError, got %T", err)
			}
			if !contains(de.Error(), c.want) {
				t.Errorf("error %q missing %q", de.Error(), c.want)
			}
		})
	}
}

func TestCompileValidFeatureAndPattern(t *testing.T) {
	d := linkDef()
	d.Features = []string{"canonicalize", "simple_get_filter"}
	ty, err := Compile(d)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !ty.HasFeature("canonicalize") || !ty.HasFeature("simple_get_filter") {
		t.Error("features not recorded")
	}
	if len(ty.Namevars()) != 2 {
		t.Errorf("namevars = %v", ty.Namevars())
	}
}

func TestDefinitionErrorMessages(t *testing.T) {
	e1 := &DefinitionError{Msg: "boom"}
	if e1.Error() != "resourceapi: invalid type definition: boom" {
		t.Errorf("got %q", e1.Error())
	}
	e2 := &DefinitionError{Type: "person", Msg: "boom"}
	if !contains(e2.Error(), `type "person"`) {
		t.Errorf("got %q", e2.Error())
	}
}

func TestValidationErrorMessages(t *testing.T) {
	e1 := &ValidationError{Type: "person", Msg: "no namevar"}
	if e1.Error() != "person: no namevar" {
		t.Errorf("got %q", e1.Error())
	}
	e2 := &ValidationError{Type: "person", Attribute: "age", Msg: "bad"}
	if e2.Error() != "person.age: bad" {
		t.Errorf("got %q", e2.Error())
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Register(Definition{Name: "Bad"}); err == nil {
		t.Fatal("expected compile error")
	}
	ty, err := r.Register(personDef())
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got, ok := r.Get("person"); !ok || got != ty {
		t.Error("Get person failed")
	}
	if _, ok := r.Get("missing"); ok {
		t.Error("Get missing should fail")
	}
	if _, err := r.Register(personDef()); err == nil {
		t.Fatal("expected duplicate error")
	}
	if names := r.Names(); len(names) != 1 || names[0] != "person" {
		t.Errorf("Names = %v", names)
	}
}

func TestGlobalRegistry(t *testing.T) {
	d := personDef()
	d.Name = "globalperson"
	if _, err := RegisterType(d); err != nil {
		t.Fatalf("RegisterType: %v", err)
	}
	if _, ok := Lookup("globalperson"); !ok {
		t.Error("Lookup failed")
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("Lookup nope should fail")
	}
}

func TestItoa(t *testing.T) {
	for _, c := range []struct {
		in   int
		want string
	}{{0, "0"}, {123, "123"}, {-5, "-5"}} {
		if got := itoa(c.in); got != c.want {
			t.Errorf("itoa(%d) = %q want %q", c.in, got, c.want)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
