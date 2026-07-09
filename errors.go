// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import "fmt"

// DefinitionError reports a malformed type [Definition] rejected by [Compile].
// The gem raises Puppet::DevError for the equivalent schema problems.
type DefinitionError struct {
	// Type is the name of the offending definition (may be empty if the name
	// itself is invalid).
	Type string
	// Msg explains what is wrong with the schema.
	Msg string
}

func (e *DefinitionError) Error() string {
	if e.Type == "" {
		return "resourceapi: invalid type definition: " + e.Msg
	}
	return fmt.Sprintf("resourceapi: invalid definition for type %q: %s", e.Type, e.Msg)
}

// ValidationError reports a resource instance that does not satisfy its type. It
// mirrors the gem's Puppet::ResourceError family; Attribute names the offending
// attribute (empty for whole-resource problems such as a missing namevar).
type ValidationError struct {
	// Type is the resource type name.
	Type string
	// Attribute is the offending attribute name, or "" for a resource-level
	// problem.
	Attribute string
	// Msg is the human-readable explanation.
	Msg string
}

func (e *ValidationError) Error() string {
	if e.Attribute == "" {
		return fmt.Sprintf("%s: %s", e.Type, e.Msg)
	}
	return fmt.Sprintf("%s.%s: %s", e.Type, e.Attribute, e.Msg)
}
