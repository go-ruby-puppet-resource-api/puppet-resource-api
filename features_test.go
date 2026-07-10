// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import (
	"errors"
	"testing"
)

// insyncDef builds a widget-like type with a custom_insync hook. The hook
// handles the "color" property case-insensitively and defers everything else.
func insyncDef(hook func(*Context, string, string, Resource, Resource) (bool, bool, error)) Definition {
	return Definition{
		Name: "cbox",
		Attributes: map[string]Attribute{
			"id":     {Type: "String", Behaviour: Namevar},
			"color":  {Type: "Optional[String]"},
			"ensure": {Type: "Enum['present','absent']", HasDefault: true, Default: "present"},
		},
		Features:     []string{FeatureCustomInsync},
		CustomInsync: hook,
	}
}

func caseInsyncHook(_ *Context, _, prop string, is, should Resource) (bool, bool, error) {
	if prop != "color" {
		return false, false, nil // defer to default deep-equal
	}
	isv, _ := is[prop].(string)
	sv, _ := should[prop].(string)
	return toLower(isv) == toLower(sv), true, nil
}

func TestCustomInsyncInSync(t *testing.T) {
	ty := mustCompile(t, insyncDef(caseInsyncHook))
	mem := newMem()
	mem.store["w"] = Resource{"id": "w", "color": "RED", "ensure": "present"}
	p := SimpleProvider{Crud: mem}
	// "red" vs "RED" -> the hook declares them in sync -> no change.
	sum, err := ty.Apply(NewContext(ty, nil), p, []Resource{{"id": "w", "color": "red"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(sum.Unchanged) != 1 {
		t.Fatalf("expected unchanged, got %+v", sum)
	}
}

func TestCustomInsyncOutOfSync(t *testing.T) {
	ty := mustCompile(t, insyncDef(caseInsyncHook))
	mem := newMem()
	mem.store["w"] = Resource{"id": "w", "color": "RED", "ensure": "present"}
	p := SimpleProvider{Crud: mem}
	// "blue" vs "RED" -> hook declares them out of sync -> update.
	sum, err := ty.Apply(NewContext(ty, nil), p, []Resource{{"id": "w", "color": "blue"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(sum.Updated) != 1 {
		t.Fatalf("expected update, got %+v", sum)
	}
}

func TestCustomInsyncFallThrough(t *testing.T) {
	// A hook that defers every property (handled=false) reproduces the default
	// deep-equal: a differing non-color attribute is still detected via the
	// default path.
	defer0 := func(_ *Context, _, _ string, _, _ Resource) (bool, bool, error) {
		return false, false, nil
	}
	ty := mustCompile(t, insyncDef(defer0))
	mem := newMem()
	mem.store["w"] = Resource{"id": "w", "color": "green", "ensure": "present"}
	p := SimpleProvider{Crud: mem}
	sum, err := ty.Apply(NewContext(ty, nil), p, []Resource{{"id": "w", "color": "yellow"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(sum.Updated) != 1 {
		t.Fatalf("expected update via default compare, got %+v", sum)
	}
}

func TestCustomInsyncError(t *testing.T) {
	boom := func(_ *Context, _, prop string, _, _ Resource) (bool, bool, error) {
		if prop == "color" {
			return false, false, errors.New("insync boom")
		}
		return false, false, nil
	}
	ty := mustCompile(t, insyncDef(boom))
	mem := newMem()
	mem.store["w"] = Resource{"id": "w", "color": "x", "ensure": "present"}
	p := SimpleProvider{Crud: mem}
	_, err := ty.Apply(NewContext(ty, nil), p, []Resource{{"id": "w", "color": "y"}})
	if err == nil || !contains(err.Error(), "insync boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestSimpleGetFilterUsesFilteredGet(t *testing.T) {
	d := personDef()
	d.Features = []string{FeatureSimpleGetFilter}
	ty := mustCompile(t, d)
	fm := newFilteredMem()
	fm.store["alice"] = Resource{"name": "alice", "created": "t1"}
	p := SimpleProvider{Crud: fm}
	_, err := ty.Apply(NewContext(ty, nil), p, []Resource{{"name": "alice", "created": "t1"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(fm.filteredWith) != 1 || fm.filteredWith[0] != "alice" {
		t.Fatalf("GetFiltered names = %v", fm.filteredWith)
	}
}

func TestSimpleGetFilterFallThroughWhenNotFilterProvider(t *testing.T) {
	// Feature declared but the provider is not a FilterProvider -> plain Get.
	d := linkDef()
	d.Features = []string{FeatureSimpleGetFilter}
	ty := mustCompile(t, d)
	bp := &bareProvider{}
	if _, err := ty.Apply(NewContext(ty, nil), bp, []Resource{{TitleKey: "a:b"}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !bp.got {
		t.Fatal("plain Get was not called")
	}
}

func TestSimpleProviderGetFilteredFallback(t *testing.T) {
	// A SimpleProvider over a non-filtered Crud falls back to a full Get.
	ty := mustCompile(t, personDef())
	mem := newMem()
	mem.store["a"] = Resource{"name": "a"}
	p := SimpleProvider{Crud: mem}
	got, err := p.GetFiltered(NewContext(ty, nil), []string{"a"})
	if err != nil || len(got) != 1 {
		t.Fatalf("GetFiltered = %v %v", got, err)
	}
	// A SimpleProvider over a FilteredCrud delegates.
	fm := newFilteredMem()
	fm.store["a"] = Resource{"name": "a"}
	pf := SimpleProvider{Crud: fm}
	if _, err := pf.GetFiltered(NewContext(ty, nil), []string{"a"}); err != nil {
		t.Fatalf("GetFiltered filtered: %v", err)
	}
	if len(fm.filteredWith) != 1 {
		t.Fatalf("delegation failed: %v", fm.filteredWith)
	}
}

func TestSupportsNoopDispatchesNoop(t *testing.T) {
	d := personDef()
	d.Features = []string{FeatureSupportsNoop}
	ty := mustCompile(t, d)
	np := &noopProvider{}
	ctx := NewContext(ty, nil).SetNoop(true)
	if ctx.Noop() != true {
		t.Fatal("SetNoop/Noop mismatch")
	}
	sum, err := ty.Apply(ctx, np, []Resource{{"name": "a", "created": "t"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(sum.Created) != 1 {
		t.Fatalf("expected create reported, got %+v", sum)
	}
	if !np.noopCalled || !np.sawNoop {
		t.Fatalf("SetNoop(noop=true) not honored: %+v", np)
	}
	if np.setCalled {
		t.Fatal("plain Set should not have been called")
	}
}

func TestSupportsNoopWithoutNoopProvider(t *testing.T) {
	// supports_noop declared but provider only implements Set -> plain Set.
	d := linkDef()
	d.Features = []string{FeatureSupportsNoop}
	ty := mustCompile(t, d)
	bp := &bareProvider{}
	if _, err := ty.Apply(NewContext(ty, nil), bp, []Resource{{TitleKey: "a:b"}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if bp.setc == nil {
		t.Fatal("Set was not called")
	}
}

func TestNoopRunWithoutSupportsNoopSkipsSet(t *testing.T) {
	// No supports_noop feature + noop context -> the change set is reported but
	// Set is not invoked.
	ty := mustCompile(t, linkDef())
	bp := &bareProvider{}
	ctx := NewContext(ty, nil).SetNoop(true)
	sum, err := ty.Apply(ctx, bp, []Resource{{TitleKey: "a:b"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(sum.Created) != 1 || len(sum.Changes) != 1 {
		t.Fatalf("change set should still be reported: %+v", sum)
	}
	if bp.setc != nil {
		t.Fatal("Set must not run in a plain noop run")
	}
}

func TestSimpleProviderSetAbsentAbsentNoop(t *testing.T) {
	ty := mustCompile(t, personDef())
	ctx := NewContext(ty, nil)
	mem := newMem()
	p := SimpleProvider{Crud: mem}
	// Both is and should absent -> nothing happens.
	if err := p.Set(ctx, map[string]Change{"x": {}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if len(mem.ops) != 0 {
		t.Fatalf("no op expected, got %v", mem.ops)
	}
}

func TestEnsureValNonStringEnsure(t *testing.T) {
	// A non-string ensure value is treated as present (asString fails).
	if got := ensureVal(Resource{"ensure": 5}); got != Present {
		t.Errorf("ensureVal(int ensure) = %q", got)
	}
	if got := ensureVal(Resource{"ensure": "absent"}); got != Absent {
		t.Errorf("ensureVal(absent) = %q", got)
	}
	if got := ensureVal(nil); got != Absent {
		t.Errorf("ensureVal(nil) = %q", got)
	}
	if got := ensureVal(Resource{"name": "x"}); got != Present {
		t.Errorf("ensureVal(no ensure key) = %q", got)
	}
	// A sensitive ensure value is unwrapped by asString.
	if got := ensureVal(Resource{"ensure": NewSensitive("absent")}); got != Absent {
		t.Errorf("ensureVal(sensitive absent) = %q", got)
	}
}
