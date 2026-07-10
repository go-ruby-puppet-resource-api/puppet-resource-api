// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

// RedactedString is the placeholder substituted for a sensitive value whenever
// it is rendered for humans (logs, error messages, [Redact]). It mirrors the
// text Puppet uses for Puppet::Pops::Types::PSensitiveType::Sensitive.
const RedactedString = "[redacted]"

// Sensitive wraps a value whose content must never be revealed by String or by
// ordinary rendering, mirroring the gem's automatic wrapping of attributes
// declared with `sensitive: true` (Puppet's Sensitive data type). The wrapped
// value stays reachable through [Sensitive.Unwrap] for the provider code that
// legitimately needs it. Equality (see [equalAny]) compares the unwrapped
// values, so a sensitive attribute still detects real changes without leaking
// its content.
type Sensitive struct {
	value any
}

// NewSensitive wraps v. Wrapping an already-[Sensitive] value returns it
// unchanged so double-wrapping is a no-op, matching the gem.
func NewSensitive(v any) *Sensitive {
	if s, ok := v.(*Sensitive); ok {
		return s
	}
	return &Sensitive{value: v}
}

// Unwrap returns the wrapped value.
func (s *Sensitive) Unwrap() any { return s.value }

// String returns the redaction placeholder, never the wrapped value, so a
// sensitive value cannot leak through fmt or logging.
func (s *Sensitive) String() string { return RedactedString }

// GoString mirrors String so %#v also redacts.
func (s *Sensitive) GoString() string { return RedactedString }

// unwrapSensitive returns the underlying value if v is [*Sensitive], else v
// unchanged, together with whether it was wrapped.
func unwrapSensitive(v any) (any, bool) {
	if s, ok := v.(*Sensitive); ok {
		return s.value, true
	}
	return v, false
}

// Redact returns a shallow copy of r in which every attribute the type declares
// sensitive — and every value already wrapped in [*Sensitive] — is replaced by
// [RedactedString], so the result is safe to log. r itself is not modified.
func (t *Type) Redact(r Resource) Resource {
	out := make(Resource, len(r))
	for k, v := range r {
		if _, ok := v.(*Sensitive); ok {
			out[k] = RedactedString
			continue
		}
		if ca, ok := t.attrs[k]; ok && ca.spec.Sensitive {
			out[k] = RedactedString
			continue
		}
		out[k] = v
	}
	return out
}
