<!--
SPDX-License-Identifier: BSD-3-Clause
Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors
-->

# Benchmarks

Standing rule: the pure-Go implementation must be **at least as fast as the
reference** — MRI's `puppet-resource_api` gem — on the hot paths that Puppet
exercises repeatedly during a catalog run: desired-state **validation** and the
**apply** (get → change-detection → set) loop.

## What is measured

`go test -bench` covers the paths a real run hammers:

| Benchmark                  | Path exercised                                                            |
|----------------------------|---------------------------------------------------------------------------|
| `BenchmarkValidate`        | title derivation + defaults + munge + Pcore type-check + custom validate  |
| `BenchmarkValidateSensitive` | the above plus sensitive-value wrapping                                 |
| `BenchmarkParseTitle`      | multi-pattern / multi-capture `title_patterns` resolution                 |
| `BenchmarkApplyUpdate`     | `Validate` + fetch + `inSync` change-detection + `SimpleProvider` dispatch |
| `BenchmarkApplyNoChange`   | the in-sync fast path (no provider write)                                  |

These mirror the gem's `Puppet::ResourceApi::Property#validate` / `#munge`,
`SimpleProvider#set` and the `title_patterns` resolution in
`Puppet::ResourceApi`.

## Reproduce (Go side)

```sh
GOWORK=off go test -run '^$' -bench=. -benchmem ./...
```

## Reference (MRI) harness

Run the equivalent gem paths in a debian **Tart VM** (per the house rule; never
Alpine, and amd64-AVX2 timing needs a full x86_64 VM):

```sh
tart run debian-bench &
gem install puppet puppet-resource_api
ruby resource_api_bench.rb   # wraps register_type + a validate/apply loop in Benchmark.realtime
```

`resource_api_bench.rb` registers the same `person` schema (one namevar, a
ranged Integer, an Optional String with munge+validate, an Enum default, a
read_only, an init_only, a parameter and `ensure`), then times N iterations of
`type.new(params)` (which drives munge/validate/type-check) and a
`SimpleProvider#set` round, using `Benchmark.realtime` divided by N for ns/op.

## Results

Go, measured on this workstation (`darwin/arm64`, Apple Silicon, Go 1.26.4).
The MRI column is captured on the Tart VM with the harness above (per the
standing rule); it is **pending real-HW capture** in this branch and must be
filled from the VM run before the rule is considered discharged — the numbers
are not guessed here.

| Benchmark                    | Go ns/op | Go B/op | Go allocs/op | MRI ns/op (Tart VM) |
|------------------------------|---------:|--------:|-------------:|---------------------:|
| `BenchmarkValidate`          |    ~700  |   712   |      6       | _pending_            |
| `BenchmarkValidateSensitive` |    ~413  |   688   |      5       | _pending_            |
| `BenchmarkParseTitle`        |    ~284  |   450   |      4       | _pending_            |
| `BenchmarkApplyUpdate`       |   ~1528  |  1112   |     11       | _pending_            |
| `BenchmarkApplyNoChange`     |   ~1663  |  1112   |     11       | _pending_            |

### Expectation

MRI's path is interpreted Ruby over `Puppet::Pops` type inference and object
allocation per attribute; the pure-Go path is a compiled type-check over
`github.com/go-pcore/pcore` with a handful of map allocations. The sub-µs Go
validation is expected to beat interpreted MRI by a wide margin; the point of
the table is to **confirm** parity-or-better against the real gem on the VM,
not to claim a number that was not measured. Fill the MRI column from the Tart
VM run and, if any Go path is slower, treat it as a regression to close.
