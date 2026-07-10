// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import "testing"

// multiPatternDef mirrors the gem's package example: a package name that may or
// may not carry a "-manager" suffix, resolved by two ordered patterns whose
// first match wins. The first pattern captures two namevars, the second one.
func multiPatternDef() Definition {
	return Definition{
		Name: "pkg",
		Attributes: map[string]Attribute{
			"package": {Type: "String", Behaviour: Namevar},
			"manager": {Type: "String", Behaviour: Namevar},
		},
		TitlePatterns: []TitlePattern{
			{Pattern: `^(?P<package>.*[^-])-(?P<manager>.*)$`, Desc: "package-manager"},
			{Pattern: `^(?P<package>.*)$`, Desc: "package only"},
		},
	}
}

func TestParseTitleSingleNamevarDefault(t *testing.T) {
	ty := mustCompile(t, personDef())
	got, err := ty.ParseTitle("alice")
	if err != nil {
		t.Fatalf("ParseTitle: %v", err)
	}
	if got["name"] != "alice" || len(got) != 1 {
		t.Errorf("ParseTitle = %v", got)
	}
}

func TestParseTitleMultiCapture(t *testing.T) {
	ty := mustCompile(t, multiPatternDef())
	// First pattern matches: two namevars decomposed.
	got, err := ty.ParseTitle("mysql-apt")
	if err != nil {
		t.Fatalf("ParseTitle: %v", err)
	}
	if got["package"] != "mysql" || got["manager"] != "apt" {
		t.Errorf("first pattern = %v", got)
	}
	// Second pattern matches when there is no hyphen: only package captured,
	// manager left for the manifest.
	got, err = ty.ParseTitle("nohyphen")
	if err != nil {
		t.Fatalf("ParseTitle: %v", err)
	}
	if got["package"] != "nohyphen" {
		t.Errorf("second pattern = %v", got)
	}
	if _, ok := got["manager"]; ok {
		t.Errorf("manager should be unset, got %v", got)
	}
}

func TestParseTitleNoMatch(t *testing.T) {
	// A pattern set that cannot match the empty string is impossible here, so
	// use a link whose single pattern needs a colon.
	ty := mustCompile(t, linkDef())
	_, err := ty.ParseTitle("nocolon")
	if err == nil || !contains(err.Error(), "no set of title patterns matched") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseTitleMultiNamevarNoPatterns(t *testing.T) {
	ty := mustCompile(t, Definition{
		Name: "pair",
		Attributes: map[string]Attribute{
			"left":  {Type: "String", Behaviour: Namevar},
			"right": {Type: "String", Behaviour: Namevar},
		},
	})
	_, err := ty.ParseTitle("x")
	if err == nil || !contains(err.Error(), "requires title_patterns") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateMultiPatternDecompose(t *testing.T) {
	ty := mustCompile(t, multiPatternDef())
	got, err := ty.Validate(Resource{TitleKey: "redis-yum"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got["package"] != "redis" || got["manager"] != "yum" {
		t.Errorf("decompose = %v", got)
	}
}

func TestValidateMultiNamevarNoPatternExplicitTitle(t *testing.T) {
	// A multi-namevar type without title_patterns but given an explicit title
	// cannot decompose it and must fail via ParseTitle in Validate.
	ty := mustCompile(t, Definition{
		Name: "pair",
		Attributes: map[string]Attribute{
			"left":  {Type: "String", Behaviour: Namevar},
			"right": {Type: "String", Behaviour: Namevar},
		},
	})
	_, err := ty.Validate(Resource{TitleKey: "x"})
	if err == nil || !contains(err.Error(), "requires title_patterns") {
		t.Fatalf("err = %v", err)
	}
}

func TestFeatureHelpers(t *testing.T) {
	d := widgetDef(lower, true)
	d.Features = []string{
		FeatureCanonicalize, FeatureCustomInsync, FeatureSimpleGetFilter,
		FeatureSupportsNoop, FeatureRemoteResource,
	}
	d.CustomInsync = func(_ *Context, _, _ string, _, _ Resource) (bool, bool, error) {
		return true, true, nil
	}
	ty := mustCompile(t, d)
	if !ty.Canonicalizes() {
		t.Error("Canonicalizes")
	}
	if !ty.CustomInsyncs() {
		t.Error("CustomInsyncs")
	}
	if !ty.SimpleGetFilter() {
		t.Error("SimpleGetFilter")
	}
	if !ty.SupportsNoop() {
		t.Error("SupportsNoop")
	}
	if !ty.RemoteResource() {
		t.Error("RemoteResource")
	}

	// Bare type declares none of them.
	bare := mustCompile(t, widgetDef(nil, false))
	if bare.Canonicalizes() || bare.CustomInsyncs() || bare.SimpleGetFilter() ||
		bare.SupportsNoop() || bare.RemoteResource() {
		t.Error("bare type should declare no features")
	}
	// Feature declared but no hook -> Canonicalizes/CustomInsyncs stay false.
	hookless := mustCompile(t, func() Definition {
		x := widgetDef(nil, true)
		x.Features = []string{FeatureCustomInsync}
		return x
	}())
	if hookless.Canonicalizes() {
		t.Error("no hook -> not Canonicalizes")
	}
	if hookless.CustomInsyncs() {
		t.Error("no hook -> not CustomInsyncs")
	}
}

func TestCanonicalizeMethod(t *testing.T) {
	// Feature + hook -> transforms.
	ty := mustCompile(t, widgetDef(lower, true))
	out, err := ty.Canonicalize(NewContext(ty, nil), []Resource{{"id": "w", "color": "RED"}})
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if out[0]["color"] != "red" {
		t.Errorf("not canonicalized: %v", out)
	}
	// No feature -> passthrough.
	ty2 := mustCompile(t, widgetDef(lower, false))
	out2, _ := ty2.Canonicalize(NewContext(ty2, nil), []Resource{{"id": "w", "color": "RED"}})
	if out2[0]["color"] != "RED" {
		t.Errorf("should be untouched: %v", out2)
	}
}

func TestSensitiveAttributes(t *testing.T) {
	ty := mustCompile(t, sensitiveDef())
	sa := ty.SensitiveAttributes()
	if len(sa) != 1 || sa[0] != "password" {
		t.Errorf("SensitiveAttributes = %v", sa)
	}
	if len(mustCompile(t, personDef()).SensitiveAttributes()) != 0 {
		t.Error("person has no sensitive attributes")
	}
}
