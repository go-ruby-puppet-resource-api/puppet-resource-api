// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import (
	"errors"
	"testing"
)

func TestLogLevelString(t *testing.T) {
	for l, want := range map[LogLevel]string{
		Debug: "debug", Info: "info", Notice: "notice",
		Warning: "warning", Err: "err", LogLevel(99): "unknown",
	} {
		if l.String() != want {
			t.Errorf("LogLevel(%d).String() = %q want %q", l, l.String(), want)
		}
	}
}

func TestContext(t *testing.T) {
	ty := mustCompile(t, personDef())

	// nil logger becomes DiscardLogger and must not panic.
	NewContext(ty, nil).Debug("swallowed")
	DiscardLogger{}.Log(Info, "swallowed")

	rl := &recLogger{}
	ctx := NewContext(ty, rl)
	if ctx.Type() != ty {
		t.Error("Type mismatch")
	}
	if ctx.Feature("canonicalize") {
		t.Error("unexpected feature")
	}
	ctx.Debug("d")
	ctx.Info("i")
	ctx.Notice("n")
	ctx.Warning("w")
	ctx.Err("e")
	ctx.Log(Debug, "l")
	if len(rl.lines) != 6 {
		t.Fatalf("lines = %v", rl.lines)
	}
	if rl.lines[0] != "debug:d" || rl.lines[4] != "err:e" {
		t.Errorf("lines = %v", rl.lines)
	}
}

func TestSimpleProviderSetErrors(t *testing.T) {
	ty := mustCompile(t, personDef())
	ctx := NewContext(ty, nil)
	should := Resource{"name": "a"}
	is := Resource{"name": "a"}

	for _, c := range []struct {
		op      string
		changes map[string]Change
	}{
		{"create", map[string]Change{"a": {Should: should}}},
		{"update", map[string]Change{"a": {Is: is, Should: should}}},
		{"delete", map[string]Change{"a": {Is: is}}},
	} {
		mem := newMem()
		mem.failOn = c.op
		p := SimpleProvider{Crud: mem}
		if err := p.Set(ctx, c.changes); err == nil {
			t.Errorf("%s: expected error", c.op)
		}
	}
}

func TestSimpleProviderGetDelegates(t *testing.T) {
	ty := mustCompile(t, personDef())
	ctx := NewContext(ty, nil)
	mem := newMem()
	mem.store["a"] = Resource{"name": "a"}
	p := SimpleProvider{Crud: mem}
	got, err := p.Get(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("Get = %v %v", got, err)
	}
}

func TestApplyLifecycle(t *testing.T) {
	ty := mustCompile(t, personDef())
	mem := newMem()
	p := SimpleProvider{Crud: mem}
	ctx := NewContext(ty, &recLogger{})

	// Round 1: create alice and bob.
	sum, err := ty.Apply(ctx, p, []Resource{
		{"name": "alice", "created": "t1"},
		{"name": "bob", "created": "t1"},
	})
	if err != nil {
		t.Fatalf("round1: %v", err)
	}
	if len(sum.Created) != 2 || len(mem.store) != 2 {
		t.Fatalf("round1 created = %v store = %d", sum.Created, len(mem.store))
	}

	// Round 2: update alice (add email), bob left untouched (not in desired).
	sum, err = ty.Apply(ctx, p, []Resource{{"name": "alice", "email": "a@b.com", "created": "t1"}})
	if err != nil {
		t.Fatalf("round2: %v", err)
	}
	if len(sum.Updated) != 1 || sum.Updated[0] != "alice" {
		t.Fatalf("round2 updated = %v", sum.Updated)
	}
	if _, ok := mem.store["bob"]; !ok {
		t.Fatal("bob was purged")
	}

	// Round 3: an init_only change is rejected.
	_, err = ty.Apply(ctx, p, []Resource{{"name": "alice", "created": "t2", "email": "a@b.com"}})
	if err == nil || !contains(err.Error(), "init_only") {
		t.Fatalf("round3 err = %v", err)
	}

	// Round 4: delete alice via ensure=absent.
	sum, err = ty.Apply(ctx, p, []Resource{{"name": "alice", "ensure": "absent"}})
	if err != nil {
		t.Fatalf("round4: %v", err)
	}
	if len(sum.Deleted) != 1 || sum.Deleted[0] != "alice" {
		t.Fatalf("round4 deleted = %v", sum.Deleted)
	}
	if _, ok := mem.store["alice"]; ok {
		t.Fatal("alice not deleted")
	}

	// Round 5: re-apply bob identically -> unchanged, provider Set not invoked.
	opsBefore := len(mem.ops)
	sum, err = ty.Apply(ctx, p, []Resource{{"name": "bob", "created": "t1"}})
	if err != nil {
		t.Fatalf("round5: %v", err)
	}
	if len(sum.Unchanged) != 1 || sum.Unchanged[0] != "bob" {
		t.Fatalf("round5 unchanged = %v", sum.Unchanged)
	}
	if len(sum.Changes) != 0 {
		t.Fatalf("round5 changes = %v", sum.Changes)
	}
	if len(mem.ops) != opsBefore {
		t.Fatal("Set was invoked for a no-op run")
	}
}

func TestApplyEnsureAbsentWhenAbsent(t *testing.T) {
	ty := mustCompile(t, personDef())
	mem := newMem()
	p := SimpleProvider{Crud: mem}
	ctx := NewContext(ty, nil)
	sum, err := ty.Apply(ctx, p, []Resource{{"name": "ghost", "ensure": "absent"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(sum.Unchanged) != 1 || len(sum.Changes) != 0 {
		t.Fatalf("sum = %+v", sum)
	}
}

func TestApplyGetError(t *testing.T) {
	ty := mustCompile(t, personDef())
	mem := newMem()
	mem.failOn = "get"
	if _, err := ty.Apply(NewContext(ty, nil), SimpleProvider{Crud: mem}, nil); err == nil {
		t.Fatal("expected get error")
	}
}

func TestApplyValidateError(t *testing.T) {
	ty := mustCompile(t, personDef())
	_, err := ty.Apply(NewContext(ty, nil), SimpleProvider{Crud: newMem()},
		[]Resource{{"name": "a", "age": 999}})
	if err == nil {
		t.Fatal("expected validate error")
	}
}

func TestApplyDesiredTitleError(t *testing.T) {
	ty := mustCompile(t, linkDef())
	// Namevars provided directly (validate passes) but no title -> Title fails
	// for a multi-namevar type when Apply keys the desired set.
	_, err := ty.Apply(NewContext(ty, nil), SimpleProvider{Crud: newMem()},
		[]Resource{{"source": "a", "target": "b"}})
	if err == nil {
		t.Fatal("expected title error")
	}
}

func TestApplyCurrentTitleError(t *testing.T) {
	ty := mustCompile(t, personDef())
	mem := newMem()
	mem.store["bad"] = Resource{"age": 1} // no namevar, no title
	_, err := ty.Apply(NewContext(ty, nil), SimpleProvider{Crud: mem}, nil)
	if err == nil {
		t.Fatal("expected current title error")
	}
}

func TestApplySetError(t *testing.T) {
	ty := mustCompile(t, personDef())
	mem := newMem()
	mem.failOn = "create"
	_, err := ty.Apply(NewContext(ty, nil), SimpleProvider{Crud: mem},
		[]Resource{{"name": "a", "created": "t"}})
	if err == nil {
		t.Fatal("expected set error")
	}
}

func TestEnsurePresentNoEnsureAttr(t *testing.T) {
	ty := mustCompile(t, linkDef()) // link has no ensure attribute
	mem := newMem()
	p := SimpleProvider{Crud: mem}
	ctx := NewContext(ty, nil)
	sum, err := ty.Apply(ctx, p, []Resource{{TitleKey: "a:b"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(sum.Created) != 1 {
		t.Fatalf("created = %v", sum.Created)
	}
}

// widgetDef builds a type whose canonicalize hook is supplied by the caller.
func widgetDef(canon func(ctx *Context, rs []Resource) ([]Resource, error), feature bool) Definition {
	d := Definition{
		Name: "widget",
		Attributes: map[string]Attribute{
			"id":     {Type: "String", Behaviour: Namevar},
			"color":  {Type: "Optional[String]"},
			"ensure": {Type: "Enum['present','absent']", HasDefault: true, Default: "present"},
		},
		Canonicalize: canon,
	}
	if feature {
		d.Features = []string{"canonicalize"}
	}
	return d
}

func lower(_ *Context, rs []Resource) ([]Resource, error) {
	for _, r := range rs {
		if c, ok := r["color"].(string); ok {
			r["color"] = toLower(c)
		}
	}
	return rs, nil
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func TestApplyCanonicalizeUnchanged(t *testing.T) {
	ty := mustCompile(t, widgetDef(lower, true))
	mem := newMem()
	mem.store["w1"] = Resource{"id": "w1", "color": "RED", "ensure": "present"}
	p := SimpleProvider{Crud: mem}
	sum, err := ty.Apply(NewContext(ty, nil), p, []Resource{{"id": "w1", "color": "red"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(sum.Unchanged) != 1 {
		t.Fatalf("expected unchanged, got %+v", sum)
	}
}

func TestApplyCanonicalizeCountMismatch(t *testing.T) {
	canon := func(_ *Context, rs []Resource) ([]Resource, error) {
		return append(rs, Resource{"id": "extra"}), nil
	}
	ty := mustCompile(t, widgetDef(canon, true))
	mem := newMem()
	p := SimpleProvider{Crud: mem}
	_, err := ty.Apply(NewContext(ty, nil), p, []Resource{{"id": "w1"}})
	if err == nil || !contains(err.Error(), "different number") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyCanonicalizeDesiredError(t *testing.T) {
	canon := func(_ *Context, rs []Resource) ([]Resource, error) {
		if len(rs) == 0 {
			return rs, nil
		}
		return nil, errors.New("boom")
	}
	ty := mustCompile(t, widgetDef(canon, true))
	p := SimpleProvider{Crud: newMem()}
	_, err := ty.Apply(NewContext(ty, nil), p, []Resource{{"id": "w1"}})
	if err == nil || !contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyCanonicalizeCurrentError(t *testing.T) {
	canon := func(_ *Context, rs []Resource) ([]Resource, error) {
		if len(rs) == 0 {
			return rs, nil
		}
		return nil, errors.New("boom")
	}
	ty := mustCompile(t, widgetDef(canon, true))
	mem := newMem()
	mem.store["w1"] = Resource{"id": "w1", "ensure": "present"}
	p := SimpleProvider{Crud: mem}
	// Empty desired -> desired canonicalize sees [] (ok); current is non-empty
	// and the hook errors there.
	_, err := ty.Apply(NewContext(ty, nil), p, nil)
	if err == nil || !contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyCanonicalizeTitleError(t *testing.T) {
	canon := func(_ *Context, rs []Resource) ([]Resource, error) {
		return []Resource{{"color": "x"}}, nil // no id/title
	}
	ty := mustCompile(t, widgetDef(canon, true))
	p := SimpleProvider{Crud: newMem()}
	_, err := ty.Apply(NewContext(ty, nil), p, []Resource{{"id": "w1"}})
	if err == nil {
		t.Fatal("expected title error from canonicalize output")
	}
}

func TestApplyCanonicalizeSkippedWithoutFeature(t *testing.T) {
	// Hook present but feature not declared -> canonicalize is skipped, so the
	// case-sensitive colors differ and an update is produced.
	ty := mustCompile(t, widgetDef(lower, false))
	mem := newMem()
	mem.store["w1"] = Resource{"id": "w1", "color": "RED", "ensure": "present"}
	p := SimpleProvider{Crud: mem}
	sum, err := ty.Apply(NewContext(ty, nil), p, []Resource{{"id": "w1", "color": "red"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(sum.Updated) != 1 {
		t.Fatalf("expected update, got %+v", sum)
	}
}

func TestApplyEqualResourcesPresenceDiffers(t *testing.T) {
	// current has color; desired omits it -> aok != bok path -> update.
	ty := mustCompile(t, widgetDef(nil, false))
	mem := newMem()
	mem.store["w2"] = Resource{"id": "w2", "color": "blue", "ensure": "present"}
	p := SimpleProvider{Crud: mem}
	sum, err := ty.Apply(NewContext(ty, nil), p, []Resource{{"id": "w2"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(sum.Updated) != 1 {
		t.Fatalf("expected update, got %+v", sum)
	}
}

func TestApplyCurrentMissingEnsure(t *testing.T) {
	// A current resource lacking the ensure key is treated as present.
	ty := mustCompile(t, widgetDef(nil, false))
	mem := newMem()
	mem.store["w4"] = Resource{"id": "w4", "color": "blue"} // no ensure key
	p := SimpleProvider{Crud: mem}
	sum, err := ty.Apply(NewContext(ty, nil), p, []Resource{{"id": "w4", "color": "red"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(sum.Updated) != 1 {
		t.Fatalf("expected update of present resource, got %+v", sum)
	}
}

func TestApplyInitOnlyOmittedInDesired(t *testing.T) {
	// An update whose desired state omits the init_only attribute must not be
	// rejected (nothing is being changed on that attribute).
	ty := mustCompile(t, personDef())
	mem := newMem()
	p := SimpleProvider{Crud: mem}
	ctx := NewContext(ty, nil)
	if _, err := ty.Apply(ctx, p, []Resource{{"name": "a", "created": "t1", "email": "a@b.com"}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	sum, err := ty.Apply(ctx, p, []Resource{{"name": "a", "email": "c@d.com"}})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(sum.Updated) != 1 {
		t.Fatalf("expected update, got %+v", sum)
	}
}

func TestEqualAnyNormalize(t *testing.T) {
	if !equalAny(int8(1), int64(1)) {
		t.Error("int8")
	}
	if !equalAny(int16(1), int(1)) {
		t.Error("int16")
	}
	if !equalAny(int32(1), 1) {
		t.Error("int32")
	}
	if !equalAny(uint(1), 1) {
		t.Error("uint")
	}
	if !equalAny(uint8(1), int64(1)) {
		t.Error("uint8")
	}
	if !equalAny(uint16(1), int64(1)) {
		t.Error("uint16")
	}
	if !equalAny(uint32(1), int64(1)) {
		t.Error("uint32")
	}
	if !equalAny(uint64(1), int64(1)) {
		t.Error("uint64")
	}
	if !equalAny(float32(1.5), 1.5) {
		t.Error("float32")
	}
	if !equalAny(float64(2.5), 2.5) {
		t.Error("float64")
	}
	if !equalAny([]any{1, int64(2)}, []any{int64(1), 2}) {
		t.Error("slice")
	}
	if !equalAny(map[string]any{"a": 1}, map[string]any{"a": int64(1)}) {
		t.Error("map")
	}
	if !equalAny(true, true) {
		t.Error("bool default")
	}
	if equalAny("x", "y") {
		t.Error("strings should differ")
	}
}
