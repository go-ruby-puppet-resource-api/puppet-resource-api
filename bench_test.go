// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import "testing"

// benchCompile compiles d or fails the benchmark.
func benchCompile(b *testing.B, d Definition) *Type {
	b.Helper()
	ty, err := Compile(d)
	if err != nil {
		b.Fatalf("Compile(%s): %v", d.Name, err)
	}
	return ty
}

// BenchmarkValidate measures the desired-state validation hot path: title
// derivation, default application, munge, type-check and custom validate.
func BenchmarkValidate(b *testing.B) {
	ty := benchCompile(b, personDef())
	in := Resource{"name": "alice", "email": "Alice@Example.COM", "role": "admin", "created": "t1"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ty.Validate(in); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidateSensitive measures validation of a type with a sensitive
// property (adds the wrap step).
func BenchmarkValidateSensitive(b *testing.B) {
	ty := benchCompile(b, sensitiveDef())
	in := Resource{"name": "db", "password": "s3cr3t"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ty.Validate(in); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseTitle measures multi-pattern, multi-capture title resolution.
func BenchmarkParseTitle(b *testing.B) {
	ty := benchCompile(b, multiPatternDef())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ty.ParseTitle("mysql-apt"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkApplyUpdate measures a full apply run that produces one update:
// validate + fetch + change detection (inSync) + dispatch.
func BenchmarkApplyUpdate(b *testing.B) {
	ty := benchCompile(b, personDef())
	mem := newMem()
	mem.store["alice"] = Resource{"name": "alice", "age": 30, "role": "user", "ensure": "present", "created": "t1"}
	p := SimpleProvider{Crud: mem}
	ctx := NewContext(ty, nil)
	desired := []Resource{{"name": "alice", "age": 31, "created": "t1"}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ty.Apply(ctx, p, desired); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkApplyNoChange measures the in-sync fast path (no provider write).
func BenchmarkApplyNoChange(b *testing.B) {
	ty := benchCompile(b, personDef())
	mem := newMem()
	mem.store["alice"] = Resource{"name": "alice", "age": 30, "role": "user", "ensure": "present", "created": "t1"}
	p := SimpleProvider{Crud: mem}
	ctx := NewContext(ty, nil)
	desired := []Resource{{"name": "alice", "age": 30, "role": "user", "created": "t1"}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ty.Apply(ctx, p, desired); err != nil {
			b.Fatal(err)
		}
	}
}
