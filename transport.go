// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import (
	"sort"

	"github.com/go-pcore/pcore"
)

// Connection is a live transport connection object. It is opaque to this
// package: the actual I/O is host-side, so a consumer (rbgo, a device provider)
// supplies whatever concrete type it likes. It mirrors the object the gem's
// context.device returns.
type Connection = any

// TransportSchema is the schema passed to [RegisterTransport], mirroring the
// hash accepted by Puppet::ResourceApi::Transport.register. It describes how to
// reach a remote device: a set of typed connection_info attributes, their
// preferred order and a host-side connect seam.
type TransportSchema struct {
	// Name is the transport name; it must match [a-z][a-z0-9_]*.
	Name string
	// Desc documents the transport. It is required, like the gem's :desc.
	Desc string
	// ConnectionInfo maps a connection attribute name to its schema (type,
	// default, munge/validate seams, sensitive flag). At least one is required.
	ConnectionInfo map[string]Attribute
	// ConnectionInfoOrder is the preferred order of the connection attributes;
	// when empty it defaults to sorted attribute names. Every entry must be a
	// declared connection attribute.
	ConnectionInfoOrder []string
	// Connect is the host-side seam that opens a [Connection] from validated
	// connection_info. It is the seam a Ruby transport class binds to; the
	// package itself performs no I/O.
	Connect func(ctx *Context, info Resource) (Connection, error)
}

// compiledTransportAttr is a connection [Attribute] with its Pcore type parsed.
type compiledTransportAttr struct {
	name string
	spec Attribute
	typ  pcore.Type
}

// Transport is a compiled, validated transport schema. It validates
// connection_info like a resource type and opens connections through the
// schema's connect seam. It is safe for concurrent use.
type Transport struct {
	schema TransportSchema
	attrs  map[string]compiledTransportAttr
	order  []string
}

// boltAttrs are the Bolt-injected connection keys the gem strips from
// connection_info when the transport schema does not itself declare them
// (clean_bolt_attributes).
var boltAttrs = map[string]bool{
	"uri": true, "host": true, "protocol": true, "user": true,
	"port": true, "password": true, "run-as": true, "shell-command": true,
	"tty": true, "connect-timeout": true, "disconnect-timeout": true,
}

// CompileTransport validates a [TransportSchema] and returns the corresponding
// [Transport]. It returns a [*DefinitionError] for any schema problem.
func CompileTransport(s TransportSchema) (*Transport, error) {
	if !typeNameRE.MatchString(s.Name) {
		return nil, &DefinitionError{Type: s.Name, Msg: "transport name must match [a-z][a-z0-9_]*"}
	}
	if s.Desc == "" {
		return nil, &DefinitionError{Type: s.Name, Msg: "transport desc is required"}
	}
	if len(s.ConnectionInfo) == 0 {
		return nil, &DefinitionError{Type: s.Name, Msg: "transport requires at least one connection_info attribute"}
	}

	tr := &Transport{schema: s, attrs: make(map[string]compiledTransportAttr, len(s.ConnectionInfo))}
	for name, spec := range s.ConnectionInfo {
		if spec.Type == "" {
			return nil, &DefinitionError{Type: s.Name, Msg: "connection_info " + name + ": type is required"}
		}
		pt, err := pcore.Parse(spec.Type)
		if err != nil {
			return nil, &DefinitionError{Type: s.Name, Msg: "connection_info " + name + ": invalid type " + spec.Type + ": " + err.Error()}
		}
		if spec.HasDefault && !pcore.IsInstance(pt, spec.Default) {
			return nil, &DefinitionError{Type: s.Name, Msg: "connection_info " + name + ": default is not a " + pt.String()}
		}
		tr.attrs[name] = compiledTransportAttr{name: name, spec: spec, typ: pt}
		tr.order = append(tr.order, name)
	}
	sort.Strings(tr.order)

	for _, n := range s.ConnectionInfoOrder {
		if _, ok := tr.attrs[n]; !ok {
			return nil, &DefinitionError{Type: s.Name, Msg: "connection_info_order references unknown attribute " + n}
		}
	}
	return tr, nil
}

// Name returns the transport name.
func (tr *Transport) Name() string { return tr.schema.Name }

// ConnectionInfoNames returns the connection attribute names in the schema's
// declared order when given, else sorted.
func (tr *Transport) ConnectionInfoNames() []string {
	if len(tr.schema.ConnectionInfoOrder) > 0 {
		out := make([]string, len(tr.schema.ConnectionInfoOrder))
		copy(out, tr.schema.ConnectionInfoOrder)
		return out
	}
	out := make([]string, len(tr.order))
	copy(out, tr.order)
	return out
}

// cleanBoltAttributes drops Bolt-injected keys the schema does not declare,
// mirroring the gem's clean_bolt_attributes. It returns a new map.
func (tr *Transport) cleanBoltAttributes(info Resource) Resource {
	out := make(Resource, len(info))
	for k, v := range info {
		if _, declared := tr.attrs[k]; !declared && boltAttrs[k] {
			continue
		}
		out[k] = v
	}
	return out
}

// Validate checks connection_info against the transport schema and returns a
// fully-populated copy: Bolt-injected keys are stripped, defaults applied, munge
// and validate seams run, every value type-checked and sensitive values wrapped.
// It mirrors the gem's transport validate step and rejects unknown attributes.
func (tr *Transport) Validate(info Resource) (Resource, error) {
	out := tr.cleanBoltAttributes(info)

	for k := range out {
		if _, ok := tr.attrs[k]; !ok {
			return nil, &ValidationError{Type: tr.schema.Name, Attribute: k, Msg: "unknown connection_info attribute"}
		}
	}

	for _, ca := range tr.attrs {
		if !ca.spec.HasDefault {
			continue
		}
		if _, set := out[ca.name]; !set {
			out[ca.name] = ca.spec.Default
		}
	}

	for _, name := range tr.order {
		ca := tr.attrs[name]
		v, set := out[name]
		if !set {
			continue
		}
		raw, wasWrapped := unwrapSensitive(v)
		v = raw
		if ca.spec.Munge != nil {
			mv, err := ca.spec.Munge(v)
			if err != nil {
				return nil, &ValidationError{Type: tr.schema.Name, Attribute: name, Msg: "munge failed: " + err.Error()}
			}
			v = mv
		}
		if !pcore.IsInstance(ca.typ, v) {
			return nil, &ValidationError{Type: tr.schema.Name, Attribute: name, Msg: "value does not match type " + ca.typ.String()}
		}
		if ca.spec.Validate != nil {
			if err := ca.spec.Validate(v); err != nil {
				return nil, &ValidationError{Type: tr.schema.Name, Attribute: name, Msg: "validation failed: " + err.Error()}
			}
		}
		if ca.spec.Sensitive || wasWrapped {
			out[name] = NewSensitive(v)
		} else {
			out[name] = v
		}
	}
	return out, nil
}

// Connect validates connection_info and opens a [Connection] through the
// schema's connect seam. The returned context (via [NewDeviceContext]) is how a
// remote_resource provider reaches the device. Connect returns a
// [*ValidationError] when the schema declares no connect seam.
func (tr *Transport) Connect(ctx *Context, info Resource) (Connection, error) {
	vi, err := tr.Validate(info)
	if err != nil {
		return nil, err
	}
	if tr.schema.Connect == nil {
		return nil, &ValidationError{Type: tr.schema.Name, Msg: "transport has no connect seam"}
	}
	return tr.schema.Connect(ctx, vi)
}

// RegisterTransport compiles s and stores the resulting [Transport], returning
// an error if the schema is invalid or a transport with the same name is
// already registered.
func (r *Registry) RegisterTransport(s TransportSchema) (*Transport, error) {
	tr, err := CompileTransport(s)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.transports[tr.schema.Name]; ok {
		return nil, &DefinitionError{Type: tr.schema.Name, Msg: "transport is already registered"}
	}
	r.transports[tr.schema.Name] = tr
	return tr, nil
}

// GetTransport returns the registered transport with the given name.
func (r *Registry) GetTransport(name string) (*Transport, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tr, ok := r.transports[name]
	return tr, ok
}

// TransportNames returns the registered transport names in sorted order.
func (r *Registry) TransportNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.transports))
	for n := range r.transports {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// RegisterTransport compiles s and registers it in the package-global registry.
func RegisterTransport(s TransportSchema) (*Transport, error) {
	return defaultRegistry.RegisterTransport(s)
}

// LookupTransport returns a transport from the package-global registry.
func LookupTransport(name string) (*Transport, bool) { return defaultRegistry.GetTransport(name) }
