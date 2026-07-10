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
// feature checks, logging, the noop flag and — for a device/transport provider
// — the transport connection, mirroring Puppet::ResourceApi::BaseContext and
// its device subclass.
type Context struct {
	typ       *Type
	logger    Logger
	noop      bool
	transport *Transport
	device    Connection
}

// NewContext builds a [Context] for the given type. A nil logger is replaced
// with [DiscardLogger].
func NewContext(t *Type, logger Logger) *Context {
	if logger == nil {
		logger = DiscardLogger{}
	}
	return &Context{typ: t, logger: logger}
}

// NewDeviceContext builds a [Context] bound to a transport and its live
// connection, mirroring the device-provider context the gem hands a
// remote_resource provider. conn is the host-side connection object (opaque to
// this package). A nil logger is replaced with [DiscardLogger].
func NewDeviceContext(t *Type, logger Logger, transport *Transport, conn Connection) *Context {
	c := NewContext(t, logger)
	c.transport = transport
	c.device = conn
	return c
}

// Type returns the resource type the context is bound to.
func (c *Context) Type() *Type { return c.typ }

// Feature reports whether the type declares the named feature.
func (c *Context) Feature(name string) bool { return c.typ.HasFeature(name) }

// Noop reports whether the run is a no-op (report-only) run.
func (c *Context) Noop() bool { return c.noop }

// SetNoop sets the noop flag and returns the context for chaining.
func (c *Context) SetNoop(noop bool) *Context { c.noop = noop; return c }

// Transport returns the transport schema the context is bound to, or nil for a
// local run.
func (c *Context) Transport() *Transport { return c.transport }

// Device returns the live transport connection, or nil for a local run. It
// mirrors the gem's context.device.
func (c *Context) Device() Connection { return c.device }

// HasDevice reports whether the context carries a transport connection.
func (c *Context) HasDevice() bool { return c.device != nil }

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

// FilterProvider is the optional get-with-names contract a [Provider] may also
// satisfy. When the type declares the simple_get_filter feature, [Apply] calls
// GetFiltered with the titles it is about to manage instead of Get, mirroring
// the gem's my_provider.get(context, names).
type FilterProvider interface {
	GetFiltered(ctx *Context, names []string) ([]Resource, error)
}

// NoopProvider is the optional noop-aware set contract a [Provider] may satisfy.
// When the type declares the supports_noop feature, [Apply] calls SetNoop with
// the context's noop flag instead of Set, mirroring the gem's
// my_provider.set(context, changes, noop:).
type NoopProvider interface {
	SetNoop(ctx *Context, changes map[string]Change, noop bool) error
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

// FilteredCrud is a [CrudProvider] that also supports fetching only the named
// instances; a [SimpleProvider] wrapping one exposes [SimpleProvider.GetFiltered]
// against it.
type FilteredCrud interface {
	CrudProvider
	GetFiltered(ctx *Context, names []string) ([]Resource, error)
}

// SimpleProvider adapts a [CrudProvider] to the [Provider] interface by turning
// each [Change] into a create, update or delete decided from the ensure values
// of the current and desired states, exactly like the gem's
// Puppet::ResourceApi::SimpleProvider: absent->present creates, present->present
// updates and present->absent deletes; absent->absent is a no-op.
type SimpleProvider struct {
	// Crud is the wrapped provider.
	Crud CrudProvider
}

// Get delegates to the wrapped provider.
func (s SimpleProvider) Get(ctx *Context) ([]Resource, error) { return s.Crud.Get(ctx) }

// GetFiltered delegates to the wrapped provider's filtered get when it supports
// one, otherwise falls back to a full Get. It lets a [SimpleProvider] satisfy
// [FilterProvider] for the simple_get_filter feature.
func (s SimpleProvider) GetFiltered(ctx *Context, names []string) ([]Resource, error) {
	if fc, ok := s.Crud.(FilteredCrud); ok {
		return fc.GetFiltered(ctx, names)
	}
	return s.Crud.Get(ctx)
}

// ensureVal returns the ensure state of a resource: [Absent] when r is nil or
// carries ensure == "absent", otherwise [Present]. A resource without an ensure
// key is treated as present, matching a manage-if-declared type.
func ensureVal(r Resource) string {
	if r == nil {
		return Absent
	}
	if v, ok := r[EnsureAttr]; ok {
		if s, ok2 := asString(v); ok2 && s == Absent {
			return Absent
		}
	}
	return Present
}

// Set turns each change into a create/update/delete based on the ensure values
// of Is and Should.
func (s SimpleProvider) Set(ctx *Context, changes map[string]Change) error {
	for _, name := range sortedKeys(changes) {
		ch := changes[name]
		isE := ensureVal(ch.Is)
		shE := ensureVal(ch.Should)
		switch {
		case isE == Absent && shE == Present:
			ctx.Notice("creating " + name)
			if err := s.Crud.Create(ctx, name, ch.Should); err != nil {
				return err
			}
		case isE == Present && shE == Present:
			ctx.Notice("updating " + name)
			if err := s.Crud.Update(ctx, name, ch.Should); err != nil {
				return err
			}
		case isE == Present && shE == Absent:
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
	return ensureVal(r) != Absent
}

// Apply drives a full management run for the desired resources against provider
// p: it validates and keys desired by title, fetches current state (via a
// filtered get when simple_get_filter is declared), canonicalizes both sides,
// computes the change set honoring ensure, the init_only behaviour and any
// custom_insync hook, hands it to p.Set (or SetNoop under supports_noop, or
// nothing under a plain noop run) and returns a [Summary].
func (t *Type) Apply(ctx *Context, p Provider, desired []Resource) (Summary, error) {
	var sum Summary

	// Validate desired and key by title first, so the managed names are known
	// before the (possibly filtered) fetch.
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

	current, err := t.fetch(ctx, p, sortedResourceKeys(desiredByName))
	if err != nil {
		return sum, err
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
	if t.Canonicalizes() {
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
			sync, err := t.inSync(ctx, name, c, d)
			if err != nil {
				return sum, err
			}
			if sync {
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
	if err := t.dispatch(ctx, p, changes); err != nil {
		return sum, err
	}
	return sum, nil
}

// fetch retrieves current state, using a filtered get when the type declares
// simple_get_filter and the provider supports one.
func (t *Type) fetch(ctx *Context, p Provider, names []string) ([]Resource, error) {
	if t.SimpleGetFilter() {
		if fp, ok := p.(FilterProvider); ok {
			return fp.GetFiltered(ctx, names)
		}
	}
	return p.Get(ctx)
}

// dispatch hands the change set to the provider, honoring supports_noop and a
// plain noop run.
func (t *Type) dispatch(ctx *Context, p Provider, changes map[string]Change) error {
	if t.SupportsNoop() {
		if np, ok := p.(NoopProvider); ok {
			return np.SetNoop(ctx, changes, ctx.Noop())
		}
		return p.Set(ctx, changes)
	}
	if ctx.Noop() {
		// Report-only run: the change set is reported in the Summary but not
		// applied, mirroring `set(...) unless noop?`.
		return nil
	}
	return p.Set(ctx, changes)
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

// inSync reports whether the managed attributes of current (is) already match
// desired (should). Only attributes declared on the type are compared;
// parameters are included because they influence the provider. When the type
// declares the custom_insync feature and supplies a [Definition.CustomInsync]
// hook, that hook decides each property, overriding the default deep-equal (a
// hook returning handled==false falls through to the default). It mirrors the
// gem's per-property insync? resolution.
func (t *Type) inSync(ctx *Context, name string, is, should Resource) (bool, error) {
	synced := true
	for _, an := range t.order {
		if t.CustomInsyncs() {
			insync, handled, err := t.def.CustomInsync(ctx, name, an, is, should)
			if err != nil {
				return false, err
			}
			if handled {
				if !insync {
					synced = false
				}
				continue
			}
		}
		isv, iok := is[an]
		sv, sok := should[an]
		if iok != sok {
			synced = false
			continue
		}
		if iok && !equalAny(isv, sv) {
			synced = false
		}
	}
	return synced, nil
}
