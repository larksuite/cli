// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package citation

import (
	"testing"

	internalcitation "github.com/larksuite/cli/internal/citation"
)

// TestTimeMatchesHostNormalization pins the SDK helper to the host's
// timestamp normalization so extension authors get the same RFC3339 output
// the built-in shortcuts emit.
func TestTimeMatchesHostNormalization(t *testing.T) {
	for _, in := range []any{"1721996760", "2026-07-27 21:26", "", nil, "not a time"} {
		if got, want := Time(in), internalcitation.Time(in); got != want {
			t.Fatalf("Time(%v) = %q, want %q", in, got, want)
		}
	}
	if got := Time("1721996760"); got == "" {
		t.Fatal("Time(unix seconds) returned empty")
	}
}
