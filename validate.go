// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import "github.com/go-pcore/pcore"

// Title derives the resource title from r: the explicit [TitleKey] if present,
// otherwise the value of the single namevar. For a multi-namevar type an
// explicit title is required.
func (t *Type) Title(r Resource) (string, error) {
	if v, ok := r[TitleKey]; ok {
		s, ok := v.(string)
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
		s, ok := v.(string)
		if !ok {
			return "", &ValidationError{Type: t.def.Name, Attribute: t.namevars[0], Msg: "namevar must be a string to serve as title"}
		}
		return s, nil
	}
	return "", &ValidationError{Type: t.def.Name, Msg: "multi-namevar type requires an explicit title"}
}

// applyTitlePatterns fills missing namevars in r by matching title against the
// compiled patterns, in order. It reports whether any namevar remained unset.
func (t *Type) applyTitlePatterns(r Resource, title string) {
	for _, p := range t.patterns {
		m := p.re.FindStringSubmatch(title)
		if m == nil {
			continue
		}
		names := p.re.SubexpNames()
		for i, name := range names {
			if name == "" {
				continue
			}
			if _, set := r[name]; !set {
				r[name] = m[i]
			}
		}
		return
	}
}

// Validate checks a desired-state resource against the type and returns a new,
// fully-populated resource: missing namevars are derived from the title, missing
// attributes with defaults are filled, munge seams run, every value is checked
// against its Pcore type and custom validate seams run. It returns a
// [*ValidationError] on the first problem.
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

	// Derive namevars from the title when possible.
	if title, err := t.Title(out); err == nil {
		t.applyTitlePatterns(out, title)
		// A single namevar always equals the title when not otherwise set.
		if len(t.namevars) == 1 {
			if _, set := out[t.namevars[0]]; !set {
				out[t.namevars[0]] = title
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

	// Munge, type-check and custom-validate every present attribute.
	for _, name := range t.order {
		ca := t.attrs[name]
		v, set := out[name]
		if !set {
			continue
		}
		if ca.spec.Munge != nil {
			mv, err := ca.spec.Munge(v)
			if err != nil {
				return nil, &ValidationError{Type: t.def.Name, Attribute: name, Msg: "munge failed: " + err.Error()}
			}
			v = mv
			out[name] = mv
		}
		if !pcore.IsInstance(ca.typ, v) {
			return nil, &ValidationError{Type: t.def.Name, Attribute: name, Msg: "value does not match type " + ca.typ.String()}
		}
		if ca.spec.Validate != nil {
			if err := ca.spec.Validate(v); err != nil {
				return nil, &ValidationError{Type: t.def.Name, Attribute: name, Msg: "validation failed: " + err.Error()}
			}
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
