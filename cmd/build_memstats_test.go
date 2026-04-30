// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"runtime"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

// TestBuild_DefaultNoCompletionLeak verifies that, without any call to
// SetFlagCompletionsEnabled, repeated cmd.Build invocations do not leak
// *cobra.Command instances into cobra's package-global flag-completion map.
//
// This guards the new default (completions disabled) — if someone flips the
// zero-value back to "enabled", the per-Build memory growth observed under
// `scripts/bench_build` would resurface in production hot paths that build
// the root command without serving a completion request.
func TestBuild_DefaultNoCompletionLeak(t *testing.T) {
	if cmdutil.FlagCompletionsEnabled() {
		t.Fatalf("precondition: FlagCompletionsEnabled() = true, want false (state polluted by another test)")
	}

	snap := func() (heapMB float64, objs uint64) {
		runtime.GC()
		runtime.GC()
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return float64(m.HeapAlloc) / 1024 / 1024, m.HeapObjects
	}

	// Warm one-time caches (registry JSON decode, embed reads) so the first
	// Build's lazy allocations don't skew the per-iteration delta.
	_ = Build(context.Background(), cmdutil.InvocationContext{})
	baseMB, baseObj := snap()

	const N = 20
	for range N {
		_ = Build(context.Background(), cmdutil.InvocationContext{})
	}
	mb, obj := snap()

	deltaMB := mb - baseMB
	deltaObj := int64(obj) - int64(baseObj)
	perBuildKB := deltaMB * 1024 / float64(N)
	perBuildObj := deltaObj / int64(N)

	t.Logf("%d builds: +%.2f MB, +%d objects (%.1f KB/build, %d objs/build)",
		N, deltaMB, deltaObj, perBuildKB, perBuildObj)

	// With completions disabled (the default), per-Build retained growth
	// should be minimal. Threshold is conservative: the previously observed
	// leak with completions enabled was ~hundreds of KB and thousands of
	// objects per Build, well above this bound.
	const maxKBPerBuild = 50.0
	const maxObjsPerBuild = 500
	if perBuildKB > maxKBPerBuild {
		t.Errorf("per-build heap growth = %.1f KB, want <= %.1f KB (completion registration may be leaking)", perBuildKB, maxKBPerBuild)
	}
	if perBuildObj > maxObjsPerBuild {
		t.Errorf("per-build object growth = %d, want <= %d", perBuildObj, maxObjsPerBuild)
	}
}
