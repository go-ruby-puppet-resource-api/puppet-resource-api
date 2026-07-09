// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import (
	"regexp"
	"sort"
	"sync"

	"github.com/go-pcore/pcore"
)

// Resource is a resource instance represented as an attribute-name to value
// map, mirroring the Ruby hash that flows through the gem. The special key
// "title" carries the resource title when the namevar(s) are not given
// explicitly.
type Resource = map[string]any

// TitleKey is the reserved key under which a resource title is supplied.
const TitleKey = "title"

// Behaviour classifies how an [Attribute] participates in management. It mirrors
// the gem's :behaviour option.
type Behaviour string

const (
	// Property is the default: a managed, readable and writable attribute.
	Property Behaviour = ""
	// Namevar uniquely identifies an instance and must be supplied.
	Namevar Behaviour = "namevar"
	// ReadOnly may be reported by a provider but never managed; supplying it in
	// a desired resource is an error.
	ReadOnly Behaviour = "read_only"
	// Parameter is data used by the provider but not enforced on the target and
	// never fetched back.
	Parameter Behaviour = "parameter"
	// InitOnly may only be set when the resource is created; changing it on an
	// existing resource is an error.
	InitOnly Behaviour = "init_only"
)

// validBehaviour reports whether b is one of the recognised behaviours.
func validBehaviour(b Behaviour) bool {
	switch b {
	case Property, Namevar, ReadOnly, Parameter, InitOnly:
		return true
	default:
		return false
	}
}

// Attribute describes a single attribute of a resource type.
type Attribute struct {
	// Type is a Pcore type expression, e.g. "String", "Integer[0,150]" or
	// "Enum['present','absent']". It is required and is validated at compile
	// time.
	Type string
	// Desc documents the attribute.
	Desc string
	// Default is the value substituted when the attribute is absent from a
	// desired resource. It is applied only when HasDefault is true, so that a
	// nil default can be distinguished from "no default".
	Default any
	// HasDefault enables Default.
	HasDefault bool
	// Behaviour selects how the attribute participates in management.
	Behaviour Behaviour
	// Munge, when set, transforms a raw value before type validation. It is the
	// seam a Ruby munge block binds to.
	Munge func(any) (any, error)
	// Validate, when set, runs custom validation after type validation. It is
	// the seam a Ruby validate block binds to.
	Validate func(any) error
}

// TitlePattern maps a resource title onto namevar values via a regular
// expression with named capture groups; each group name must be a declared
// attribute.
type TitlePattern struct {
	// Pattern is a Go regular expression with named groups.
	Pattern string
	// Desc documents the pattern.
	Desc string
}

// Definition is the schema passed to [Compile] / [RegisterType], mirroring the
// hash accepted by Puppet::ResourceApi.register_type.
type Definition struct {
	// Name is the resource type name; it must match [a-z][a-z0-9_]*.
	Name string
	// Desc documents the type.
	Desc string
	// Attributes maps attribute names to their schemas. At least one attribute
	// with the [Namevar] behaviour is required.
	Attributes map[string]Attribute
	// TitlePatterns decompose a title into namevars when they are not supplied
	// explicitly. They are tried in order.
	TitlePatterns []TitlePattern
	// Features lists optional provider capabilities such as "canonicalize" or
	// "simple_get_filter". Unknown feature names are accepted (the gem only
	// warns); empty names are rejected.
	Features []string
	// Autorequire, Autobefore, Autonotify and Autosubscribe map a target
	// resource type name to the attribute whose value names the related
	// resource, mirroring the gem's auto-relation options.
	Autorequire   map[string]string
	Autobefore    map[string]string
	Autonotify    map[string]string
	Autosubscribe map[string]string
	// Canonicalize, when set and enabled by the "canonicalize" feature,
	// normalises both current and desired resources so they compare equal when
	// semantically identical. It is the seam a Ruby canonicalize method binds
	// to.
	Canonicalize func(ctx *Context, resources []Resource) ([]Resource, error)
}

// compiledAttr is an [Attribute] with its Pcore type parsed.
type compiledAttr struct {
	name string
	spec Attribute
	typ  pcore.Type
}

// compiledPattern is a [TitlePattern] with its regexp compiled.
type compiledPattern struct {
	re     *regexp.Regexp
	groups []string
}

// Type is a compiled, validated resource type. It is safe for concurrent use.
type Type struct {
	def      Definition
	attrs    map[string]compiledAttr
	order    []string // attribute names, sorted, for deterministic iteration
	namevars []string // namevar attribute names, sorted
	patterns []compiledPattern
	features map[string]bool
}

// Name returns the type name.
func (t *Type) Name() string { return t.def.Name }

// Definition returns a copy of the source definition.
func (t *Type) Definition() Definition { return t.def }

// AttributeNames returns the attribute names in sorted order.
func (t *Type) AttributeNames() []string {
	out := make([]string, len(t.order))
	copy(out, t.order)
	return out
}

// Namevars returns the namevar attribute names in sorted order.
func (t *Type) Namevars() []string {
	out := make([]string, len(t.namevars))
	copy(out, t.namevars)
	return out
}

// HasFeature reports whether the named feature is declared on the type.
func (t *Type) HasFeature(name string) bool { return t.features[name] }

var typeNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Compile validates a [Definition] and returns the corresponding [Type]. It
// returns a [*DefinitionError] for any schema problem.
func Compile(d Definition) (*Type, error) {
	if !typeNameRE.MatchString(d.Name) {
		return nil, &DefinitionError{Type: d.Name, Msg: "name must match [a-z][a-z0-9_]*"}
	}
	if len(d.Attributes) == 0 {
		return nil, &DefinitionError{Type: d.Name, Msg: "at least one attribute is required"}
	}

	t := &Type{
		def:      d,
		attrs:    make(map[string]compiledAttr, len(d.Attributes)),
		features: make(map[string]bool, len(d.Features)),
	}

	for name, spec := range d.Attributes {
		if spec.Type == "" {
			return nil, &DefinitionError{Type: d.Name, Msg: "attribute " + name + ": type is required"}
		}
		if !validBehaviour(spec.Behaviour) {
			return nil, &DefinitionError{Type: d.Name, Msg: "attribute " + name + ": unknown behaviour " + string(spec.Behaviour)}
		}
		pt, err := pcore.Parse(spec.Type)
		if err != nil {
			return nil, &DefinitionError{Type: d.Name, Msg: "attribute " + name + ": invalid type " + spec.Type + ": " + err.Error()}
		}
		if spec.HasDefault && !pcore.IsInstance(pt, spec.Default) {
			return nil, &DefinitionError{Type: d.Name, Msg: "attribute " + name + ": default is not a " + pt.String()}
		}
		t.attrs[name] = compiledAttr{name: name, spec: spec, typ: pt}
		t.order = append(t.order, name)
		if spec.Behaviour == Namevar {
			t.namevars = append(t.namevars, name)
		}
	}
	sort.Strings(t.order)
	sort.Strings(t.namevars)

	if len(t.namevars) == 0 {
		return nil, &DefinitionError{Type: d.Name, Msg: "at least one namevar attribute is required"}
	}

	for i, f := range d.Features {
		if f == "" {
			return nil, &DefinitionError{Type: d.Name, Msg: "feature at index " + itoa(i) + " is empty"}
		}
		t.features[f] = true
	}

	// Auto-relations reference attribute names.
	for _, rel := range []map[string]string{d.Autorequire, d.Autobefore, d.Autonotify, d.Autosubscribe} {
		for target, attr := range rel {
			if _, ok := t.attrs[attr]; !ok {
				return nil, &DefinitionError{Type: d.Name, Msg: "auto-relation to " + target + " references unknown attribute " + attr}
			}
		}
	}

	for i, tp := range d.TitlePatterns {
		re, err := regexp.Compile(tp.Pattern)
		if err != nil {
			return nil, &DefinitionError{Type: d.Name, Msg: "title_pattern at index " + itoa(i) + ": " + err.Error()}
		}
		groups := re.SubexpNames()
		var named []string
		for _, g := range groups {
			if g == "" {
				continue
			}
			if _, ok := t.attrs[g]; !ok {
				return nil, &DefinitionError{Type: d.Name, Msg: "title_pattern at index " + itoa(i) + ": capture " + g + " is not an attribute"}
			}
			named = append(named, g)
		}
		if len(named) == 0 {
			return nil, &DefinitionError{Type: d.Name, Msg: "title_pattern at index " + itoa(i) + ": no named captures"}
		}
		t.patterns = append(t.patterns, compiledPattern{re: re, groups: named})
	}

	return t, nil
}

// itoa is a tiny dependency-free integer formatter for error messages.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}

// Registry holds compiled types by name, mirroring Puppet's global type
// registry. It is safe for concurrent use.
type Registry struct {
	mu    sync.Mutex
	types map[string]*Type
}

// NewRegistry returns an empty [Registry].
func NewRegistry() *Registry {
	return &Registry{types: make(map[string]*Type)}
}

// Register compiles d and stores the resulting [Type], returning an error if the
// definition is invalid or a type with the same name is already registered.
func (r *Registry) Register(d Definition) (*Type, error) {
	t, err := Compile(d)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.types[t.def.Name]; ok {
		return nil, &DefinitionError{Type: t.def.Name, Msg: "type is already registered"}
	}
	r.types[t.def.Name] = t
	return t, nil
}

// Get returns the registered type with the given name.
func (r *Registry) Get(name string) (*Type, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.types[name]
	return t, ok
}

// Names returns the registered type names in sorted order.
func (r *Registry) Names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.types))
	for n := range r.types {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// defaultRegistry backs the package-level [RegisterType] / [Lookup].
var defaultRegistry = NewRegistry()

// RegisterType compiles d and registers it in the package-global registry.
func RegisterType(d Definition) (*Type, error) { return defaultRegistry.Register(d) }

// Lookup returns a type from the package-global registry.
func Lookup(name string) (*Type, bool) { return defaultRegistry.Get(name) }
