// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import (
	"errors"
	"strings"
)

// personDef is the representative type used across the tests: a namevar, a typed
// attribute with a range, an optional attribute with a munge+validate seam, an
// enum with a default, a read_only attribute, an init_only attribute, a
// parameter and ensure.
func personDef() Definition {
	return Definition{
		Name: "person",
		Desc: "a person",
		Attributes: map[string]Attribute{
			"name": {Type: "String[1]", Behaviour: Namevar, Desc: "the name"},
			"age":  {Type: "Integer[0,150]", HasDefault: true, Default: 30, Desc: "age in years"},
			"email": {
				Type: "Optional[String]",
				Desc: "email address",
				Munge: func(v any) (any, error) {
					s, ok := v.(string)
					if !ok {
						return nil, errors.New("email must be a string")
					}
					return strings.ToLower(s), nil
				},
				Validate: func(v any) error {
					if !strings.Contains(v.(string), "@") {
						return errors.New("email must contain @")
					}
					return nil
				},
			},
			"role":    {Type: "Enum['admin','user','guest']", HasDefault: true, Default: "user", Desc: "role"},
			"ssn":     {Type: "String", Behaviour: ReadOnly, Desc: "social security number"},
			"created": {Type: "String", Behaviour: InitOnly, Desc: "creation timestamp"},
			"note":    {Type: "String", Behaviour: Parameter, Desc: "operator note"},
			"ensure":  {Type: "Enum['present','absent']", HasDefault: true, Default: "present", Desc: "ensure"},
		},
		Autorequire: map[string]string{"file": "name"},
	}
}

// linkDef is a two-namevar type with a title pattern, used to exercise title
// handling.
func linkDef() Definition {
	return Definition{
		Name: "link",
		Attributes: map[string]Attribute{
			"source": {Type: "String", Behaviour: Namevar},
			"target": {Type: "String", Behaviour: Namevar},
		},
		TitlePatterns: []TitlePattern{
			{Pattern: `^(?P<source>[^:]+):(?P<target>.+)$`, Desc: "source:target"},
		},
	}
}

// recLogger records every log line for assertions.
type recLogger struct{ lines []string }

func (r *recLogger) Log(l LogLevel, msg string) {
	r.lines = append(r.lines, l.String()+":"+msg)
}

// memProvider is a CrudProvider backed by an in-memory map keyed by title. It
// records every operation and can be told to fail a chosen operation.
type memProvider struct {
	store  map[string]Resource
	ops    []string
	failOn string // "get","create","update","delete" to force an error
}

func newMem() *memProvider { return &memProvider{store: map[string]Resource{}} }

func (m *memProvider) Get(ctx *Context) ([]Resource, error) {
	if m.failOn == "get" {
		return nil, errors.New("get failed")
	}
	out := make([]Resource, 0, len(m.store))
	for _, r := range m.store {
		cp := make(Resource, len(r))
		for k, v := range r {
			cp[k] = v
		}
		out = append(out, cp)
	}
	return out, nil
}

func (m *memProvider) Create(ctx *Context, name string, should Resource) error {
	if m.failOn == "create" {
		return errors.New("create failed")
	}
	m.ops = append(m.ops, "create:"+name)
	cp := make(Resource, len(should))
	for k, v := range should {
		cp[k] = v
	}
	m.store[name] = cp
	return nil
}

func (m *memProvider) Update(ctx *Context, name string, should Resource) error {
	if m.failOn == "update" {
		return errors.New("update failed")
	}
	m.ops = append(m.ops, "update:"+name)
	cp := make(Resource, len(should))
	for k, v := range should {
		cp[k] = v
	}
	m.store[name] = cp
	return nil
}

func (m *memProvider) Delete(ctx *Context, name string) error {
	if m.failOn == "delete" {
		return errors.New("delete failed")
	}
	m.ops = append(m.ops, "delete:"+name)
	delete(m.store, name)
	return nil
}

// filteredMem is a memProvider that also records the names passed to a filtered
// get, letting SimpleProvider satisfy FilterProvider for simple_get_filter.
type filteredMem struct {
	*memProvider
	filteredWith []string
}

func newFilteredMem() *filteredMem { return &filteredMem{memProvider: newMem()} }

func (f *filteredMem) GetFiltered(ctx *Context, names []string) ([]Resource, error) {
	f.filteredWith = append([]string{}, names...)
	return f.memProvider.Get(ctx)
}

// bareProvider implements only the Provider contract (no GetFiltered), used to
// exercise the simple_get_filter fall-through when a provider is not a
// FilterProvider.
type bareProvider struct {
	got  bool
	setc map[string]Change
}

func (b *bareProvider) Get(ctx *Context) ([]Resource, error) { b.got = true; return nil, nil }
func (b *bareProvider) Set(ctx *Context, changes map[string]Change) error {
	b.setc = changes
	return nil
}

// noopProvider is a Provider that records whether Set/SetNoop ran and the noop
// flag it saw, for the supports_noop tests.
type noopProvider struct {
	setCalled  bool
	noopCalled bool
	sawNoop    bool
}

func (n *noopProvider) Get(ctx *Context) ([]Resource, error) { return nil, nil }
func (n *noopProvider) Set(ctx *Context, changes map[string]Change) error {
	n.setCalled = true
	return nil
}
func (n *noopProvider) SetNoop(ctx *Context, changes map[string]Change, noop bool) error {
	n.noopCalled = true
	n.sawNoop = noop
	return nil
}

// sensitiveDef is a type with a sensitive property, used for redaction tests.
func sensitiveDef() Definition {
	return Definition{
		Name: "secret",
		Attributes: map[string]Attribute{
			"name":     {Type: "String", Behaviour: Namevar},
			"password": {Type: "String", Sensitive: true},
			"ensure":   {Type: "Enum['present','absent']", HasDefault: true, Default: "present"},
		},
	}
}
