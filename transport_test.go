// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import (
	"errors"
	"testing"
)

// fakeConn is a stand-in for a host-side transport connection.
type fakeConn struct{ info Resource }

// sshSchema is the representative transport: a host, a defaulted port and user,
// and a sensitive password, with a connect seam returning a fakeConn.
func sshSchema() TransportSchema {
	return TransportSchema{
		Name: "ssh",
		Desc: "ssh transport",
		ConnectionInfo: map[string]Attribute{
			"host":     {Type: "String"},
			"port":     {Type: "Integer[1,65535]", HasDefault: true, Default: 22},
			"user":     {Type: "String", HasDefault: true, Default: "root"},
			"password": {Type: "String", Sensitive: true},
		},
		ConnectionInfoOrder: []string{"host", "port", "user", "password"},
		Connect: func(_ *Context, info Resource) (Connection, error) {
			return &fakeConn{info: info}, nil
		},
	}
}

func mustCompileTransport(t *testing.T, s TransportSchema) *Transport {
	t.Helper()
	tr, err := CompileTransport(s)
	if err != nil {
		t.Fatalf("CompileTransport(%s): %v", s.Name, err)
	}
	return tr
}

func TestCompileTransportSuccess(t *testing.T) {
	tr := mustCompileTransport(t, sshSchema())
	if tr.Name() != "ssh" {
		t.Errorf("Name = %q", tr.Name())
	}
	names := tr.ConnectionInfoNames()
	if len(names) != 4 || names[0] != "host" || names[3] != "password" {
		t.Errorf("ordered names = %v", names)
	}
	// No explicit order -> sorted names.
	s := sshSchema()
	s.ConnectionInfoOrder = nil
	tr2 := mustCompileTransport(t, s)
	if got := tr2.ConnectionInfoNames(); got[0] != "host" || got[1] != "password" {
		t.Errorf("sorted names = %v", got)
	}
}

func TestCompileTransportErrors(t *testing.T) {
	cases := []struct {
		name string
		mut  func(s *TransportSchema)
		want string
	}{
		{"bad name", func(s *TransportSchema) { s.Name = "Bad" }, "transport name must match"},
		{"no desc", func(s *TransportSchema) { s.Desc = "" }, "desc is required"},
		{"no conn info", func(s *TransportSchema) { s.ConnectionInfo = nil }, "at least one connection_info"},
		{"attr no type", func(s *TransportSchema) {
			s.ConnectionInfo = map[string]Attribute{"host": {}}
		}, "type is required"},
		{"attr bad type", func(s *TransportSchema) {
			s.ConnectionInfo = map[string]Attribute{"host": {Type: "Nope[["}}
		}, "invalid type"},
		{"default not instance", func(s *TransportSchema) {
			s.ConnectionInfo = map[string]Attribute{"host": {Type: "String", HasDefault: true, Default: 5}}
		}, "default is not a"},
		{"bad order", func(s *TransportSchema) { s.ConnectionInfoOrder = []string{"nope"} }, "unknown attribute"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := sshSchema()
			c.mut(&s)
			_, err := CompileTransport(s)
			if err == nil {
				t.Fatalf("expected error %q", c.want)
			}
			if de, ok := err.(*DefinitionError); !ok || !contains(de.Error(), c.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestTransportValidateDefaultsAndSensitive(t *testing.T) {
	tr := mustCompileTransport(t, sshSchema())
	got, err := tr.Validate(Resource{"host": "example.net", "password": "pw"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got["port"] != 22 || got["user"] != "root" {
		t.Errorf("defaults = %v", got)
	}
	sv, ok := got["password"].(*Sensitive)
	if !ok || sv.Unwrap() != "pw" {
		t.Errorf("password not wrapped: %v", got["password"])
	}
}

func TestTransportValidateBoltCleaning(t *testing.T) {
	tr := mustCompileTransport(t, sshSchema())
	// "uri" is a Bolt-injected key the schema does not declare -> stripped.
	// "host" is Bolt-named but declared -> kept.
	got, err := tr.Validate(Resource{"host": "h", "uri": "ssh://h", "password": "pw"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, ok := got["uri"]; ok {
		t.Error("uri should have been stripped")
	}
	if got["host"] != "h" {
		t.Error("host should be kept")
	}
}

func TestTransportValidateErrors(t *testing.T) {
	tr := mustCompileTransport(t, sshSchema())
	if _, err := tr.Validate(Resource{"host": "h", "bogus": 1}); err == nil {
		t.Error("unknown attr should error")
	}
	if _, err := tr.Validate(Resource{"host": 5}); err == nil {
		t.Error("type mismatch should error")
	}

	// munge error.
	ms := sshSchema()
	ms.ConnectionInfo["user"] = Attribute{Type: "String", Munge: func(any) (any, error) {
		return nil, errors.New("munge boom")
	}}
	if _, err := mustCompileTransport(t, ms).Validate(Resource{"host": "h", "user": "u"}); err == nil ||
		!contains(err.Error(), "munge boom") {
		t.Errorf("munge err = %v", err)
	}

	// validate-hook error.
	vs := sshSchema()
	vs.ConnectionInfo["user"] = Attribute{Type: "String", Validate: func(any) error {
		return errors.New("validate boom")
	}}
	if _, err := mustCompileTransport(t, vs).Validate(Resource{"host": "h", "user": "u"}); err == nil ||
		!contains(err.Error(), "validate boom") {
		t.Errorf("validate err = %v", err)
	}
}

func TestTransportValidateSuccessfulMunge(t *testing.T) {
	s := sshSchema()
	s.ConnectionInfo["user"] = Attribute{Type: "String", Munge: func(v any) (any, error) {
		return toLower(v.(string)), nil
	}}
	got, err := mustCompileTransport(t, s).Validate(Resource{"host": "h", "user": "ROOT", "password": "pw"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got["user"] != "root" {
		t.Errorf("munge not applied: %v", got["user"])
	}
}

func TestTransportValidatePrewrappedSensitive(t *testing.T) {
	tr := mustCompileTransport(t, sshSchema())
	got, err := tr.Validate(Resource{"host": "h", "password": NewSensitive("pw")})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got["password"].(*Sensitive).Unwrap() != "pw" {
		t.Errorf("prewrapped lost: %v", got["password"])
	}
}

func TestTransportConnect(t *testing.T) {
	tr := mustCompileTransport(t, sshSchema())
	ctx := NewContext(nil, nil)
	conn, err := tr.Connect(ctx, Resource{"host": "h", "password": "pw"})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	fc, ok := conn.(*fakeConn)
	if !ok || fc.info["host"] != "h" {
		t.Fatalf("conn = %#v", conn)
	}

	// Validation failure propagates.
	if _, err := tr.Connect(ctx, Resource{"host": 5}); err == nil {
		t.Error("bad info should fail Connect")
	}

	// No connect seam.
	ns := sshSchema()
	ns.Connect = nil
	if _, err := mustCompileTransport(t, ns).Connect(ctx, Resource{"host": "h"}); err == nil ||
		!contains(err.Error(), "no connect seam") {
		t.Errorf("no-seam err = %v", err)
	}
}

func TestTransportRegistry(t *testing.T) {
	r := NewRegistry()
	if _, err := r.RegisterTransport(TransportSchema{Name: "Bad"}); err == nil {
		t.Fatal("expected compile error")
	}
	tr, err := r.RegisterTransport(sshSchema())
	if err != nil {
		t.Fatalf("RegisterTransport: %v", err)
	}
	if got, ok := r.GetTransport("ssh"); !ok || got != tr {
		t.Error("GetTransport ssh failed")
	}
	if _, ok := r.GetTransport("missing"); ok {
		t.Error("GetTransport missing should fail")
	}
	if _, err := r.RegisterTransport(sshSchema()); err == nil {
		t.Fatal("expected duplicate error")
	}
	if names := r.TransportNames(); len(names) != 1 || names[0] != "ssh" {
		t.Errorf("TransportNames = %v", names)
	}
}

func TestGlobalTransportRegistry(t *testing.T) {
	s := sshSchema()
	s.Name = "globalssh"
	if _, err := RegisterTransport(s); err != nil {
		t.Fatalf("RegisterTransport: %v", err)
	}
	if _, ok := LookupTransport("globalssh"); !ok {
		t.Error("LookupTransport failed")
	}
	if _, ok := LookupTransport("nope"); ok {
		t.Error("LookupTransport nope should fail")
	}
}

func TestDeviceContext(t *testing.T) {
	ty := mustCompile(t, personDef())
	tr := mustCompileTransport(t, sshSchema())
	conn := &fakeConn{info: Resource{"host": "h"}}
	ctx := NewDeviceContext(ty, nil, tr, conn)
	if !ctx.HasDevice() {
		t.Error("HasDevice should be true")
	}
	if ctx.Device() != conn {
		t.Error("Device mismatch")
	}
	if ctx.Transport() != tr {
		t.Error("Transport mismatch")
	}
	// A local context has no device.
	if NewContext(ty, nil).HasDevice() {
		t.Error("local context should have no device")
	}
}
