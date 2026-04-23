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

// Name intentionally returns a value that mimics an external provider; the
// PkgPath-based classifier must ignore it. See TestIsBuiltinProvider_PkgPathNotSpoofableByName.
func (cmdutilLocalProvider) Name() string { return "external-spoofed-provider" }
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
	// which is compile-time fixed. The local type returns a name that looks
	// like an ISV provider; it must still classify as builtin.
	p := &cmdutilLocalProvider{}
	if p.Name() != "external-spoofed-provider" {
		t.Fatalf("sanity check: Name() = %q, spoof value lost", p.Name())
	}
	if !isBuiltinProvider(p) {
		t.Fatal("isBuiltinProvider should decide by PkgPath, not Name()")
	}
}

// TestIsBuiltinProvider_NonPointerValues covers the non-pointer reflect branch.
// The existing tests only exercise pointer receivers (&T{}); when a provider
// is passed by value the reflect.Kind is not Ptr and t.Elem() is skipped.
func TestIsBuiltinProvider_NonPointerValues(t *testing.T) {
	if !isBuiltinProvider(cmdutilLocalProvider{}) {
		t.Fatal("non-pointer local type should be builtin (PkgPath still under official module)")
	}
	// http.Server as a non-pointer — PkgPath "net/http", not under official.
	if isBuiltinProvider(http.Server{}) {
		t.Fatal("non-pointer stdlib type should not be builtin")
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
// classifyBuild — pure branching logic
// ---------------------------------------------------------------------------
//
// These tests cover every branch of classifyBuild with explicit inputs,
// which is impossible from computeBuildKind alone because debug.ReadBuildInfo
// and the process-wide provider registries can't be reshaped in a test.

func TestClassifyBuild_NoBuildInfo_ReturnsUnknown(t *testing.T) {
	if got := classifyBuild("", false, nil, nil, nil); got != BuildKindUnknown {
		t.Fatalf("classifyBuild(haveBuildInfo=false) = %q, want %q", got, BuildKindUnknown)
	}
}

func TestClassifyBuild_ExtendedMainPath_ReturnsExtended(t *testing.T) {
	cases := []string{
		"github.com/acme/lark-cli-wrapper",
		"example.com/isv/lark",
		"gitlab.mycorp.internal/tools/lark-cli-fork",
	}
	for _, mp := range cases {
		t.Run(mp, func(t *testing.T) {
			if got := classifyBuild(mp, true, nil, nil, nil); got != BuildKindExtended {
				t.Fatalf("mainPath=%q classifyBuild = %q, want %q", mp, got, BuildKindExtended)
			}
		})
	}
}

func TestClassifyBuild_OfficialMainPath_NoProviders_ReturnsOfficial(t *testing.T) {
	if got := classifyBuild(officialModulePath, true, nil, nil, nil); got != BuildKindOfficial {
		t.Fatalf("classifyBuild(official, no providers) = %q, want %q", got, BuildKindOfficial)
	}
}

func TestClassifyBuild_EmptyMainPath_DoesNotTriggerExtended(t *testing.T) {
	// An empty Main.Path (rare, e.g. `go run` pre-1.18) must not be treated
	// as extended by itself — the classifier falls through to provider checks.
	if got := classifyBuild("", true, nil, nil, nil); got != BuildKindOfficial {
		t.Fatalf("classifyBuild(empty mainPath, no providers) = %q, want %q", got, BuildKindOfficial)
	}
}

func TestClassifyBuild_NonBuiltinCredentialProvider_ReturnsExtended(t *testing.T) {
	// Any non-builtin credential provider flips the verdict to extended.
	got := classifyBuild(officialModulePath, true, []any{&http.Server{}}, nil, nil)
	if got != BuildKindExtended {
		t.Fatalf("classifyBuild with external credential = %q, want %q", got, BuildKindExtended)
	}
}

func TestClassifyBuild_MixedCredentialProviders_ExtendedWins(t *testing.T) {
	// Even if most providers are builtin, a single external one decides.
	providers := []any{&cmdutilLocalProvider{}, &http.Server{}}
	if got := classifyBuild(officialModulePath, true, providers, nil, nil); got != BuildKindExtended {
		t.Fatalf("classifyBuild mixed providers = %q, want %q", got, BuildKindExtended)
	}
}

func TestClassifyBuild_NonBuiltinTransportProvider_ReturnsExtended(t *testing.T) {
	got := classifyBuild(officialModulePath, true, nil, &http.Server{}, nil)
	if got != BuildKindExtended {
		t.Fatalf("classifyBuild with external transport = %q, want %q", got, BuildKindExtended)
	}
}

func TestClassifyBuild_NonBuiltinFileioProvider_ReturnsExtended(t *testing.T) {
	got := classifyBuild(officialModulePath, true, nil, nil, &http.Server{})
	if got != BuildKindExtended {
		t.Fatalf("classifyBuild with external fileio = %q, want %q", got, BuildKindExtended)
	}
}

func TestClassifyBuild_AllBuiltinProviders_ReturnsOfficial(t *testing.T) {
	// All three slots filled with builtin providers must still classify as official.
	got := classifyBuild(
		officialModulePath, true,
		[]any{&cmdutilLocalProvider{}},
		&cmdutilLocalProvider{},
		&cmdutilLocalProvider{},
	)
	if got != BuildKindOfficial {
		t.Fatalf("classifyBuild all-builtin = %q, want %q", got, BuildKindOfficial)
	}
}

// TestClassifyBuild_MainPathPriorityOverProviders documents that the main
// module path takes precedence: even with only builtin providers, a non-
// official main path still yields extended.
func TestClassifyBuild_MainPathPriorityOverProviders(t *testing.T) {
	got := classifyBuild(
		"github.com/acme/lark-wrapper", true,
		[]any{&cmdutilLocalProvider{}},
		&cmdutilLocalProvider{},
		&cmdutilLocalProvider{},
	)
	if got != BuildKindExtended {
		t.Fatalf("main-path override failed: got %q, want %q", got, BuildKindExtended)
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
