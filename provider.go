// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

// EnsureAttr is the conventional name of the ensure attribute; when a type
// declares it, [Apply] uses its value ("present"/"absent") to decide between
// create/update and delete.
const EnsureAttr = "ensure"

// Ensure values.
const (
	Present = "present"
	Absent  = "absent"
)

// LogLevel classifies a log line emitted through a [Context].
type LogLevel int

// Log levels, mirroring Puppet's logger.
const (
	Debug LogLevel = iota
	Info
	Notice
	Warning
	Err
)

// String returns the lowercase level name.
func (l LogLevel) String() string {
	switch l {
	case Debug:
		return "debug"
	case Info:
		return "info"
	case Notice:
		return "notice"
	case Warning:
		return "warning"
	case Err:
		return "err"
	default:
		return "unknown"
	}
}

// Logger receives log lines from a provider via its [Context]. It is the seam a
// Ruby logger binds to.
type Logger interface {
	Log(level LogLevel, msg string)
}

// DiscardLogger is a [Logger] that drops every line.
type DiscardLogger struct{}

// Log implements [Logger].
func (DiscardLogger) Log(LogLevel, string) {}

// Context is handed to a [Provider]'s Get and Set. It exposes the owning type,
// feature checks and logging, mirroring Puppet::ResourceApi::BaseContext.
type Context struct {
	typ    *Type
	logger Logger
}

// NewContext builds a [Context] for the given type. A nil logger is replaced
// with [DiscardLogger].
func NewContext(t *Type, logger Logger) *Context {
	if logger == nil {
		logger = DiscardLogger{}
	}
	return &Context{typ: t, logger: logger}
}

// Type returns the resource type the context is bound to.
func (c *Context) Type() *Type { return c.typ }

// Feature reports whether the type declares the named feature.
func (c *Context) Feature(name string) bool { return c.typ.HasFeature(name) }

// Log emits a line at the given level.
func (c *Context) Log(level LogLevel, msg string) { c.logger.Log(level, msg) }

// Debug logs at [Debug].
func (c *Context) Debug(msg string) { c.logger.Log(Debug, msg) }

// Info logs at [Info].
func (c *Context) Info(msg string) { c.logger.Log(Info, msg) }

// Notice logs at [Notice].
func (c *Context) Notice(msg string) { c.logger.Log(Notice, msg) }

// Warning logs at [Warning].
func (c *Context) Warning(msg string) { c.logger.Log(Warning, msg) }

// Err logs at [Err].
func (c *Context) Err(msg string) { c.logger.Log(Err, msg) }

// Change is one entry in the set of changes handed to [Provider.Set], keyed by
// title. Is is the current state (nil when the resource is absent) and Should is
// the desired state (nil when the resource should be absent).
type Change struct {
	Is     Resource
	Should Resource
}

// Provider is the get/set contract every provider implements.
type Provider interface {
	// Get returns the current state of every managed instance.
	Get(ctx *Context) ([]Resource, error)
	// Set applies the given changes, keyed by title.
	Set(ctx *Context, changes map[string]Change) error
}

// CrudProvider is the simpler contract a provider may implement instead; wrap it
// in a [SimpleProvider] to obtain a [Provider].
type CrudProvider interface {
	// Get returns the current state of every managed instance.
	Get(ctx *Context) ([]Resource, error)
	// Create makes a new resource with the given title and desired state.
	Create(ctx *Context, name string, should Resource) error
	// Update reconciles an existing resource to the desired state.
	Update(ctx *Context, name string, should Resource) error
	// Delete removes an existing resource.
	Delete(ctx *Context, name string) error
}

// SimpleProvider adapts a [CrudProvider] to the [Provider] interface by turning
// each [Change] into a create, update or delete, exactly like the gem's
// Puppet::ResourceApi::SimpleProvider.
type SimpleProvider struct {
	// Crud is the wrapped provider.
	Crud CrudProvider
}

// Get delegates to the wrapped provider.
func (s SimpleProvider) Get(ctx *Context) ([]Resource, error) { return s.Crud.Get(ctx) }

// Set turns each change into a create/update/delete based on the presence of Is
// and Should.
func (s SimpleProvider) Set(ctx *Context, changes map[string]Change) error {
	for _, name := range sortedKeys(changes) {
		ch := changes[name]
		isPresent := ch.Is != nil
		shouldPresent := ch.Should != nil
		switch {
		case shouldPresent && !isPresent:
			ctx.Notice("creating " + name)
			if err := s.Crud.Create(ctx, name, ch.Should); err != nil {
				return err
			}
		case shouldPresent && isPresent:
			ctx.Notice("updating " + name)
			if err := s.Crud.Update(ctx, name, ch.Should); err != nil {
				return err
			}
		case !shouldPresent && isPresent:
			ctx.Notice("deleting " + name)
			if err := s.Crud.Delete(ctx, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// Summary reports what an [Apply] run did. Counts are keyed by the action.
type Summary struct {
	Created   []string
	Updated   []string
	Deleted   []string
	Unchanged []string
	// Changes is the exact change set handed to the provider's Set.
	Changes map[string]Change
}

// ensurePresent reports whether resource r (for type t) is present. When the
// type has no ensure attribute every resource is considered present.
func (t *Type) ensurePresent(r Resource) bool {
	if _, ok := t.attrs[EnsureAttr]; !ok {
		return true
	}
	v, ok := r[EnsureAttr]
	if !ok {
		return true
	}
	return v != Absent
}

// Apply drives a full management run for the desired resources against provider
// p: it fetches current state, validates and canonicalizes desired against
// current, computes the change set honoring ensure and the init_only behaviour,
// hands it to p.Set and returns a [Summary].
func (t *Type) Apply(ctx *Context, p Provider, desired []Resource) (Summary, error) {
	var sum Summary

	current, err := p.Get(ctx)
	if err != nil {
		return sum, err
	}

	// Validate desired, then key by title. current is trusted (it comes from
	// the provider) but still needs a title.
	desiredByName := make(map[string]Resource, len(desired))
	for _, d := range desired {
		vd, err := t.Validate(d)
		if err != nil {
			return sum, err
		}
		name, err := t.Title(vd)
		if err != nil {
			return sum, err
		}
		desiredByName[name] = vd
	}

	currentByName := make(map[string]Resource, len(current))
	for _, c := range current {
		name, err := t.Title(c)
		if err != nil {
			return sum, err
		}
		currentByName[name] = c
	}

	// Optional canonicalization of both sides.
	if t.def.Canonicalize != nil && t.HasFeature("canonicalize") {
		if desiredByName, err = t.canonicalizeMap(ctx, desiredByName); err != nil {
			return sum, err
		}
		if currentByName, err = t.canonicalizeMap(ctx, currentByName); err != nil {
			return sum, err
		}
	}

	// Only resources named in the desired set are managed; current-only
	// resources are left untouched (no implicit purge), matching Puppet's
	// default behaviour. Deletion is requested by a desired resource whose
	// ensure is absent.
	changes := make(map[string]Change)
	for _, name := range sortedResourceKeys(desiredByName) {
		d := desiredByName[name]
		c, haveCurrent := currentByName[name]

		isPresent := haveCurrent && t.ensurePresent(c)
		shouldPresent := t.ensurePresent(d)

		switch {
		case shouldPresent && !isPresent:
			changes[name] = Change{Should: d}
			sum.Created = append(sum.Created, name)
		case shouldPresent && isPresent:
			if err := t.checkInitOnly(name, c, d); err != nil {
				return sum, err
			}
			if t.equalResources(c, d) {
				sum.Unchanged = append(sum.Unchanged, name)
				continue
			}
			changes[name] = Change{Is: c, Should: d}
			sum.Updated = append(sum.Updated, name)
		case !shouldPresent && isPresent:
			changes[name] = Change{Is: c}
			sum.Deleted = append(sum.Deleted, name)
		default:
			// Desired absent and current absent: nothing to do.
			sum.Unchanged = append(sum.Unchanged, name)
		}
	}

	sum.Changes = changes
	if len(changes) == 0 {
		return sum, nil
	}
	if err := p.Set(ctx, changes); err != nil {
		return sum, err
	}
	return sum, nil
}

// canonicalizeMap applies the type's canonicalize hook to the values of m,
// preserving keys.
func (t *Type) canonicalizeMap(ctx *Context, m map[string]Resource) (map[string]Resource, error) {
	names := sortedResourceKeys(m)
	in := make([]Resource, len(names))
	for i, n := range names {
		in[i] = m[n]
	}
	out, err := t.def.Canonicalize(ctx, in)
	if err != nil {
		return nil, err
	}
	if len(out) != len(in) {
		return nil, &ValidationError{Type: t.def.Name, Msg: "canonicalize returned a different number of resources"}
	}
	res := make(map[string]Resource, len(out))
	for _, r := range out {
		name, err := t.Title(r)
		if err != nil {
			return nil, err
		}
		res[name] = r
	}
	return res, nil
}

// checkInitOnly rejects a change to an init_only attribute on an existing
// resource.
func (t *Type) checkInitOnly(name string, is, should Resource) error {
	for _, an := range t.order {
		if t.attrs[an].spec.Behaviour != InitOnly {
			continue
		}
		sv, shas := should[an]
		if !shas {
			continue
		}
		if !equalAny(is[an], sv) {
			return &ValidationError{Type: t.def.Name, Attribute: an, Msg: "init_only attribute cannot be changed on resource " + name}
		}
	}
	return nil
}

// equalResources reports whether the managed attributes of two resources are
// equal. Only attributes declared on the type are compared; parameters are
// included because they influence the provider.
func (t *Type) equalResources(a, b Resource) bool {
	for _, name := range t.order {
		av, aok := a[name]
		bv, bok := b[name]
		if aok != bok {
			return false
		}
		if aok && !equalAny(av, bv) {
			return false
		}
	}
	return true
}
