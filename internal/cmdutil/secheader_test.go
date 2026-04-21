// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"net/http"
	"testing"

	"github.com/larksuite/cli/extension/credential"
	envcred "github.com/larksuite/cli/extension/credential/env"
	"github.com/larksuite/cli/internal/vfs/localfileio"
)

// ---------------------------------------------------------------------------
// isBuiltinProvider
// ---------------------------------------------------------------------------

// cmdutilLocalProvider has PkgPath under the official module
// ("github.com/larksuite/cli/internal/cmdutil") and should be classified
// as builtin.
type cmdutilLocalProvider struct{}

func (cmdutilLocalProvider) Name() string { return "local" }
func (cmdutilLocalProvider) ResolveAccount(context.Context) (*credential.Account, error) {
	return nil, nil
}
func (cmdutilLocalProvider) ResolveToken(context.Context, credential.TokenSpec) (*credential.Token, error) {
	return nil, nil
}

func TestIsBuiltinProvider_Nil(t *testing.T) {
	if isBuiltinProvider(nil) {
		t.Fatal("isBuiltinProvider(nil) = true, want false")
	}
}

func TestIsBuiltinProvider_TypeUnderOfficialModule(t *testing.T) {
	if !isBuiltinProvider(&cmdutilLocalProvider{}) {
		t.Fatal("type under github.com/larksuite/cli/... should be builtin")
	}
}

func TestIsBuiltinProvider_StdlibTypeIsNotBuiltin(t *testing.T) {
	// A standard library type has PkgPath "net/http" — outside official module.
	// This covers the non-builtin branch, which we cannot trigger from inside
	// this test file using a locally-defined type.
	if isBuiltinProvider(&http.Server{}) {
		t.Fatal("stdlib type classified as builtin, PkgPath check is broken")
	}
}

func TestIsBuiltinProvider_PkgPathNotSpoofableByName(t *testing.T) {
	// Name() returns a string, but classification uses reflect.Type.PkgPath
	// which is compile-time fixed. Confirm by using a locally-defined type
	// that returns a name mimicking an external provider.
	p := &cmdutilLocalProvider{}
	if p.Name() != "local" {
		t.Fatalf("sanity check: Name() = %q", p.Name())
	}
	if !isBuiltinProvider(p) {
		t.Fatal("isBuiltinProvider should decide by PkgPath, not Name()")
	}
}

// TestIsBuiltinProvider_RealBuiltinProviders locks down the classification
// for the concrete providers enumerated in design doc §3.3.2 as "官方自带":
// env credential provider and local fileio provider. If any of these is
// moved out of the official module tree in the future, this test must flip
// red so the new package path is explicitly considered.
//
// The sidecar providers (extension/credential/sidecar and
// extension/transport/sidecar) are guarded by the `authsidecar` build tag
// and covered in secheader_sidecar_test.go under that tag.
func TestIsBuiltinProvider_RealBuiltinProviders(t *testing.T) {
	cases := []struct {
		name     string
		provider any
	}{
		{"env credential provider", &envcred.Provider{}},
		{"local fileio provider", &localfileio.Provider{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !isBuiltinProvider(tc.provider) {
				t.Fatalf("%T must be classified as builtin (PkgPath under %s)", tc.provider, officialModulePath)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// computeBuildKind
// ---------------------------------------------------------------------------

func TestComputeBuildKind_ReturnsKnownValue(t *testing.T) {
	// Under `go test`, Main.Path is typically the module being tested
	// ("github.com/larksuite/cli"); the concrete return may still be
	// official, extended, or unknown depending on Main.Path and the
	// registered providers. Just assert it's one of the defined values.
	got := computeBuildKind()
	switch got {
	case BuildKindOfficial, BuildKindExtended, BuildKindUnknown:
	default:
		t.Fatalf("computeBuildKind() = %q, want one of official/extended/unknown", got)
	}
}

// ---------------------------------------------------------------------------
// DetectBuildKind — sync.Once caching
// ---------------------------------------------------------------------------

func TestDetectBuildKind_StableAcrossCalls(t *testing.T) {
	a := DetectBuildKind()
	b := DetectBuildKind()
	if a != b {
		t.Fatalf("DetectBuildKind() returned different values on repeat: %q vs %q", a, b)
	}
}

// ---------------------------------------------------------------------------
// BaseSecurityHeaders
// ---------------------------------------------------------------------------

func TestBaseSecurityHeaders_IncludesBuildHeader(t *testing.T) {
	h := BaseSecurityHeaders()
	v := h.Get(HeaderBuild)
	if v == "" {
		t.Fatal("BaseSecurityHeaders missing X-Cli-Build header")
	}
	switch v {
	case BuildKindOfficial, BuildKindExtended, BuildKindUnknown:
	default:
		t.Fatalf("X-Cli-Build = %q, want one of official/extended/unknown", v)
	}
}

func TestBaseSecurityHeaders_AllRequiredHeaders(t *testing.T) {
	h := BaseSecurityHeaders()
	for _, key := range []string{HeaderSource, HeaderVersion, HeaderBuild, HeaderUserAgent} {
		if h.Get(key) == "" {
			t.Errorf("BaseSecurityHeaders missing %s", key)
		}
	}
}
