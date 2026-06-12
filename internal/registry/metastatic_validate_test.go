// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Validation for the static-meta registry: the generated metastatic.Registry is
// the sole embedded baseline (no JSON parsed at runtime), and a deep read of it
// allocates nothing. The data is generated from meta_data.json at build time
// (`make fetch_meta`) and is gitignored, so these tests skip on a bare checkout
// where it has not been generated yet.
package registry

import (
	"testing"

	"github.com/larksuite/cli/internal/registry/metaschema"
	"github.com/larksuite/cli/internal/registry/metastatic"
)

func countFieldsStatic(fs []metaschema.Field) int {
	n := 0
	for _, f := range fs {
		n++
		n += countFieldsStatic(f.Properties)
	}
	return n
}

func countStatic() (svc, res, meth, fld int) {
	svc = len(metastatic.Registry.Services)
	for _, s := range metastatic.Registry.Services {
		for _, r := range s.Resources {
			res++
			for _, m := range r.Methods {
				meth++
				fld += countFieldsStatic(m.Parameters) + countFieldsStatic(m.RequestBody) + countFieldsStatic(m.ResponseBody)
			}
		}
	}
	return
}

// TestStaticRegistryPopulated checks the generated registry carries data. It
// skips on a bare checkout where meta_data_gen.go has not been generated yet.
func TestStaticRegistryPopulated(t *testing.T) {
	if len(metastatic.Registry.Services) == 0 {
		t.Skip("static registry empty; run `make fetch_meta` to generate it")
	}
	svc, res, meth, fld := countStatic()
	t.Logf("static: services=%d resources=%d methods=%d fields=%d", svc, res, meth, fld)
	if svc == 0 || res == 0 || meth == 0 || fld == 0 {
		t.Fatalf("static registry incomplete: svc=%d res=%d meth=%d fld=%d", svc, res, meth, fld)
	}
	if metastatic.Registry.Version == "" {
		t.Error("static registry has empty Version")
	}
}

var sinkInt int

// --- zero-alloc: a deep read of the static registry must allocate nothing ---

func deepReadStatic() int {
	n := 0
	for _, s := range metastatic.Registry.Services {
		n += len(s.Name)
		for _, r := range s.Resources {
			for _, m := range r.Methods {
				n += len(m.ID) + len(m.Scopes) + countFieldsStatic(m.Parameters) + countFieldsStatic(m.ResponseBody)
			}
		}
	}
	return n
}

func TestStaticReadZeroAlloc(t *testing.T) {
	if len(metastatic.Registry.Services) == 0 {
		t.Skip("static registry empty; run `make fetch_meta` to generate it")
	}
	avg := testing.AllocsPerRun(50, func() { sinkInt = deepReadStatic() })
	t.Logf("static deep-read: %.1f allocs/op", avg)
	if avg > 0 {
		t.Errorf("static read allocates %.1f/op, want 0 (data should be in the binary, not heap)", avg)
	}
}

func BenchmarkReadStaticRegistry(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkInt = deepReadStatic()
	}
}
