// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package shortcuts

import "testing"

// BenchmarkAllShortcuts pins the cost of the one deep copy every CLI startup
// pays. AllShortcuts is the isolation boundary for external distributions, so
// this allocation is deliberate -- but callers inside this repository receive
// an already-isolated snapshot, and re-cloning it multiplies this number by
// the number of consumers. If this benchmark regresses, look for a new clone
// on the build path before assuming the shortcut set simply grew.
func BenchmarkAllShortcuts(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = AllShortcuts()
	}
}

// BenchmarkAllShortcutsWithExternal covers the snapshot the command tree is
// actually built from, so the external-command path is measured too.
func BenchmarkAllShortcutsWithExternal(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := AllShortcutsWithExternal(nil); err != nil {
			b.Fatal(err)
		}
	}
}
