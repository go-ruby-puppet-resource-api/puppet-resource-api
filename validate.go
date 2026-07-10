// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import "github.com/go-pcore/pcore"

// Title derives the resource title from r: the explicit [TitleKey] if present,
// otherwise the value of the single namevar. For a multi-namevar type an
// explicit title is required. A namevar wrapped in [*Sensitive] is unwrapped.
func (t *Type) Title(r Resource) (string, error) {
	if v, ok := r[TitleKey]; ok {
		s, ok := asString(v)
		if !ok {
			return "", &ValidationError{Type: t.def.Name, Attribute: TitleKey, Msg: "title must be a string"}
		}
		return s, nil
	}
	if len(t.namevars) == 1 {
		v, ok := r[t.namevars[0]]
		if !ok {
			return "", &ValidationError{Type: t.def.Name, Msg: "cannot determine title: neither title nor namevar " + t.namevars[0] + " is set"}
		}
		s, ok := asString(v)
		if !ok {
			return "", &ValidationError{Type: t.def.Name, Attribute: t.namevars[0], Msg: "namevar must be a string to serve as title"}
		}
		return s, nil
	}
	return "", &ValidationError{Type: t.def.Name, Msg: "multi-namevar type requires an explicit title"}
}

// asString reports v as a string, unwrapping a [*Sensitive] first.
func asString(v any) (string, bool) {
	if u, ok := unwrapSensitive(v); ok {
		v = u
	}
	s, ok := v.(string)
	return s, ok
}

// ParseTitle decomposes a resource title into namevar values, mirroring
// Puppet::ResourceApi's title-pattern resolution. When the type declares
// [Definition.TitlePatterns] each pattern is tried in order and the named
// captures of the first one that matches become the returned attribute values;
// if none match, a [*ValidationError] is returned, exactly as the gem raises
// when no set of title patterns matches. With no declared patterns a
// single-namevar type maps the whole title to its namevar (the gem's default
// [[/(.*)/m, [[namevar]]]] pattern) and a multi-namevar type is an error.
func (t *Type) ParseTitle(title string) (map[string]string, error) {
	if len(t.patterns) > 0 {
		for _, p := range t.patterns {
			m := p.re.FindStringSubmatch(title)
			if m == nil {
				continue
			}
			out := make(map[string]string, len(p.groups))
			names := p.re.SubexpNames()
			for i, name := range names {
				if name == "" {
					continue
				}
				out[name] = m[i]
			}
			return out, nil
		}
		return nil, &ValidationError{Type: t.def.Name, Msg: "no set of title patterns matched the title " + quote(title)}
	}
	if len(t.namevars) == 1 {
		return map[string]string{t.namevars[0]: title}, nil
	}
	return nil, &ValidationError{Type: t.def.Name, Msg: "multi-namevar type requires title_patterns to decompose title " + quote(title)}
}

// quote wraps s in double quotes for error messages without pulling in fmt.
func quote(s string) string { return "\"" + s + "\"" }

// Validate checks a desired-state resource against the type and returns a new,
// fully-populated resource: missing namevars are derived from the title (via the
// title patterns), missing attributes with defaults are filled, munge seams run,
// every value is checked against its Pcore type, custom validate seams run and
// sensitive values are wrapped. It returns a [*ValidationError] on the first
// problem.
func (t *Type) Validate(input Resource) (Resource, error) {
	// Reject unknown attributes and read_only management up front, before we
	// mutate anything.
	for k := range input {
		if k == TitleKey {
			continue
		}
		ca, ok := t.attrs[k]
		if !ok {
			return nil, &ValidationError{Type: t.def.Name, Attribute: k, Msg: "unknown attribute"}
		}
		if ca.spec.Behaviour == ReadOnly {
			return nil, &ValidationError{Type: t.def.Name, Attribute: k, Msg: "attribute is read-only and cannot be managed"}
		}
	}

	// Work on a copy so the caller's map is untouched.
	out := make(Resource, len(input))
	for k, v := range input {
		out[k] = v
	}

	// Derive namevars from the title when a title is obtainable. A title is
	// obtainable for a single-namevar type or when TitleKey is given; a
	// multi-namevar type whose namevars are already supplied has no title and
	// simply skips decomposition.
	if title, err := t.Title(out); err == nil {
		parsed, perr := t.ParseTitle(title)
		if perr != nil {
			return nil, perr
		}
		for k, v := range parsed {
			if _, set := out[k]; !set {
				out[k] = v
			}
		}
	}

	// Apply defaults for absent attributes.
	for _, ca := range t.attrs {
		if !ca.spec.HasDefault {
			continue
		}
		if _, set := out[ca.name]; !set {
			out[ca.name] = ca.spec.Default
		}
	}

	// Munge, type-check, custom-validate and (last) wrap-if-sensitive every
	// present attribute, in that order.
	for _, name := range t.order {
		ca := t.attrs[name]
		v, set := out[name]
		if !set {
			continue
		}
		raw, wasWrapped := unwrapSensitive(v)
		v = raw
		if ca.spec.Munge != nil {
			mv, err := ca.spec.Munge(v)
			if err != nil {
				return nil, &ValidationError{Type: t.def.Name, Attribute: name, Msg: "munge failed: " + err.Error()}
			}
			v = mv
		}
		if !pcore.IsInstance(ca.typ, v) {
			return nil, &ValidationError{Type: t.def.Name, Attribute: name, Msg: "value does not match type " + ca.typ.String()}
		}
		if ca.spec.Validate != nil {
			if err := ca.spec.Validate(v); err != nil {
				return nil, &ValidationError{Type: t.def.Name, Attribute: name, Msg: "validation failed: " + err.Error()}
			}
		}
		if ca.spec.Sensitive || wasWrapped {
			out[name] = NewSensitive(v)
		} else {
			out[name] = v
		}
	}

	// Every namevar must now be present.
	for _, nv := range t.namevars {
		if _, set := out[nv]; !set {
			return nil, &ValidationError{Type: t.def.Name, Attribute: nv, Msg: "namevar is required"}
		}
	}

	return out, nil
}

// Canonicalize runs the type's [Definition.Canonicalize] hook over resources
// when the canonicalize feature is declared and a hook is set, returning the
// normalised resources; otherwise it returns resources unchanged. It is the
// public entry point mirroring the gem's my_provider.canonicalize call.
func (t *Type) Canonicalize(ctx *Context, resources []Resource) ([]Resource, error) {
	if !t.Canonicalizes() {
		return resources, nil
	}
	return t.def.Canonicalize(ctx, resources)
}
