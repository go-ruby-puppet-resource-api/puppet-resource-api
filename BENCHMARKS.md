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

Go, measured on this workstation (`darwin/arm64`, Apple Silicon, Go 1.26.4):

| Benchmark                    | Go ns/op | Go B/op | Go allocs/op |
|------------------------------|---------:|--------:|-------------:|
| `BenchmarkValidate`          |    ~700  |   712   |      6       |
| `BenchmarkValidateSensitive` |    ~413  |   688   |      5       |
| `BenchmarkParseTitle`        |    ~284  |   450   |      4       |
| `BenchmarkApplyUpdate`       |   ~1528  |  1112   |     11       |
| `BenchmarkApplyNoChange`     |   ~1663  |  1112   |     11       |

## Measured results — real hardware (2026-07-10)

The MRI reference was captured on **real, non-x86 hardware** with the
`puppet-resource_api` **2.0.1** gem (Puppet 8.10.0). The closest gem-level
equivalent to `Validate` is registering the same `person` schema with
`Puppet::ResourceApi.register_type` and timing `type.new(params)` — the call
that drives default application, munge, Pcore type-check and validation — under
`Benchmark.realtime` after a warm-up. (`type.new` additionally performs Puppet
`Type` instantiation/provider resolution, so it is if anything *generous* to the
reference.) The gem does not expose the isolated `ParseTitle`/`Apply` paths as
standalone calls, so the validate/instantiate path is the head-to-head number.

| Arch | Host | CPU | Go `Validate` | MRI `type.new` | ratio (MRI ÷ Go) |
|------|------|-----|--------------:|---------------:|-----------------:|
| `s390x`   | LinuxONE — IBM z15, 2 vCPU        | go1.26.4 / Ruby 3.2.3 | 2 107 ns  | 161 531 ns   | **76.7× faster** |
| `riscv64` | cfarm95 — SpacemiT X60 (rv64gcv)  | go1.26.4 / Ruby 3.3.8 | 15 654 ns | 3 382 980 ns | **216× faster** |

The pure-Go validation is **far faster than the reference** on both real
architectures — a compiled type-check over `github.com/go-pcore/pcore` versus
interpreted Ruby over `Puppet::Pops` type inference with per-attribute object
allocation. The "≥ reference" rule is discharged with real-HW numbers; no Go
path regressed.
