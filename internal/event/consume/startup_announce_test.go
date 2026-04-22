// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bytes"
	"strings"
	"testing"
)

func TestAnnounceForkedBus(t *testing.T) {
	var buf bytes.Buffer
	announceForkedBus(&buf, 12345)

	got := buf.String()
	for _, want := range []string{
		"[event] started bus daemon",
		"pid=12345",
		"auto-exits 30s after last consumer",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got:\n%s", want, got)
		}
	}
}
