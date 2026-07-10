// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

// Package resourceapi is a pure-Go (no cgo) port of the core of Puppet's
// puppet-resource_api gem — the modern type/provider API used to describe and
// manage resources.
//
// It provides three cooperating pieces that mirror the gem:
//
//   - Type definition and registration. A [Definition] describes a resource
//     type: its name, its typed [Attribute]s (each carrying a Pcore type
//     expression, an optional default, a documentation string and a
//     [Behaviour]), title patterns, features and the auto-relation maps
//     (autorequire/autobefore/autonotify/autosubscribe). [Compile] validates a
//     definition and turns it into a ready-to-use [Type]; [RegisterType] does
//     the same and stores the result in a package-global [Registry] (a private
//     [Registry] can be built with [NewRegistry] for isolated use).
//
//   - Instance validation. [Type.Validate] takes a desired-state resource
//     hash, derives missing namevars from the title (directly for a single
//     namevar, or via the compiled [TitlePattern]s), applies defaults, runs the
//     per-attribute munge seam, checks every value against its declared Pcore
//     type (using github.com/go-pcore/pcore) and runs the per-attribute custom
//     validate seam. It rejects unknown attributes, missing namevars and any
//     attempt to manage a read_only attribute, producing typed [ValidationError]
//     values whose messages track the gem where reasonable.
//
//   - The provider protocol. A [Provider] implements the get(context) ->
//     []instance / set(context, changes) contract against a [Context] (logging,
//     the owning [Type] and feature checks). [Apply] drives a full run: it calls
//     Get for the current state, validates and canonicalizes desired against
//     current, computes the per-title [Change] set honoring ensure and the
//     init_only behaviour and any custom_insync hook, then calls Set.
//     [SimpleProvider] is the base that translates that change set into
//     create/update/delete calls on a [CrudProvider], deciding each from the
//     ensure values exactly like the gem's SimpleProvider.
//
//   - Transport / device support. [RegisterTransport] compiles a
//     [TransportSchema] (typed connection_info attributes) into a [Transport]
//     that validates connection info like a type and opens a [Connection]
//     through a host-side connect seam; [NewDeviceContext] hands a
//     remote_resource provider that connection through [Context.Device].
//
// The feature flags the gem acts on are all honored: canonicalize,
// custom_insync (the per-property [Definition.CustomInsync] comparison seam that
// overrides the default deep-equal), simple_get_filter (a filtered
// [FilterProvider.GetFiltered] fetch of only the managed titles), supports_noop
// (a noop-aware [NoopProvider.SetNoop] dispatch) and remote_resource. Attributes
// declared Sensitive are wrapped in [*Sensitive] after validation so logs and
// error messages redact them ([Type.Redact]).
//
// The package is a pure library: it embeds no Ruby runtime. The interpreter
// facing hooks (munge, validate, canonicalize, custom_insync, the transport
// connect seam and the provider itself) are Go func and interface seams that a
// consumer such as go-embedded-ruby (rbgo) wires to Ruby blocks; executing the
// Ruby bodies of those blocks is that binding layer's job. Everything the gem
// specifies as behaviour — title-pattern resolution, the validate/apply pipeline
// ordering, the change model and the feature flags — lives here.
package resourceapi
