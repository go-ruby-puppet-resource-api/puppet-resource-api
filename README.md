# puppet-resource-api

[![ci](https://github.com/go-ruby-puppet-resource-api/puppet-resource-api/actions/workflows/ci.yml/badge.svg)](https://github.com/go-ruby-puppet-resource-api/puppet-resource-api/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-ruby-puppet-resource-api/puppet-resource-api.svg)](https://pkg.go.dev/github.com/go-ruby-puppet-resource-api/puppet-resource-api)
[![coverage](https://img.shields.io/badge/coverage-100%25-brightgreen)](https://github.com/go-ruby-puppet-resource-api/puppet-resource-api/actions/workflows/ci.yml)

A pure-Go (`CGO_ENABLED=0`) port of the core of Puppet's
[`puppet-resource_api`](https://github.com/puppetlabs/puppet-resource_api) gem —
the modern type/provider API used to describe and manage resources.

It provides three cooperating pieces that mirror the gem:

- **Type definition and registration.** A `Definition` describes a resource
  type: its name, its typed `Attribute`s (each carrying a Pcore type
  expression, an optional default, a doc string and a `Behaviour` — `namevar`,
  `read_only`, `parameter`, `init_only`), title patterns, features and the
  auto-relation maps. `Compile` validates a definition; `RegisterType` also
  stores it in a global `Registry`.
- **Instance validation.** `Type.Validate` derives missing namevars from the
  title (directly or via title patterns), applies defaults, runs the
  per-attribute munge seam, checks every value against its declared Pcore type
  (via [`github.com/go-pcore/pcore`](https://github.com/go-pcore/pcore)) and
  runs the per-attribute validate seam, rejecting unknown attributes, missing
  namevars and any attempt to manage a `read_only` attribute.
- **The provider protocol.** A `Provider` implements `Get(ctx) -> []instance`
  and `Set(ctx, changes)`. `Type.Apply` fetches current state, validates and
  canonicalizes desired against current, computes the per-title change set
  honoring `ensure` and `init_only`, then calls `Set`. `SimpleProvider`
  translates that change set into create/update/delete calls on a
  `CrudProvider`.

The package embeds no Ruby runtime: the munge, validate, canonicalize,
custom_insync, transport-connect and provider hooks are Go func/interface seams
a consumer such as
[go-embedded-ruby](https://github.com/go-embedded-ruby) can wire to Ruby blocks.

## Install

```sh
go get github.com/go-ruby-puppet-resource-api/puppet-resource-api@latest
```

## Example

```go
ty, _ := resourceapi.Compile(resourceapi.Definition{
    Name: "person",
    Attributes: map[string]resourceapi.Attribute{
        "name":   {Type: "String[1]", Behaviour: resourceapi.Namevar},
        "role":   {Type: "Enum['admin','user']", HasDefault: true, Default: "user"},
        "ensure": {Type: "Enum['present','absent']", HasDefault: true, Default: "present"},
    },
})
r, err := ty.Validate(resourceapi.Resource{"name": "alice", "role": "admin"})
// r == {"name":"alice", "role":"admin", "ensure":"present"}
```

## Scope

Supported: type definition + validation, all four behaviours, multi-pattern /
multi-capture `title_patterns` resolution, auto-relations, defaults, the
munge → type-check → validate pipeline with per-attribute munge/validate and
per-type canonicalize seams, the get/set provider contract and the
`SimpleProvider` create/update/delete base that decides each action from the
`ensure` values, honoring `init_only`.

Every feature flag the gem acts on is honored:

- **`canonicalize`** — the per-type normalise hook, gated on the feature.
- **`custom_insync`** — the per-property comparison seam that overrides the
  default deep-equal in change detection.
- **`simple_get_filter`** — a filtered `GetFiltered(names)` fetch of only the
  managed titles.
- **`supports_noop`** — a noop-aware `SetNoop(changes, noop)` dispatch (a plain
  noop run reports the change set without applying it).
- **`remote_resource`** — transport/device support: `RegisterTransport(schema)`
  compiles a `Transport` that validates connection info like a type and opens a
  `Connection` through a host-side connect seam; `NewDeviceContext` exposes it
  to a provider via `Context.Device()`.

Attributes declared `Sensitive` are wrapped in `*Sensitive` after validation so
logs and errors redact them (`Type.Redact`), while equality still compares the
underlying content.

Executing the Ruby bodies bound to the seams (munge/validate/canonicalize/
custom_insync/connect blocks) is the [go-embedded-ruby](https://github.com/go-embedded-ruby)
binding layer's job; the behaviour the gem specifies lives here.

Out of scope: the JSON-schema / puppet-strings documentation emitters.

## License

BSD-3-Clause. See [LICENSE](LICENSE).
