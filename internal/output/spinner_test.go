// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"strings"
	"testing"
)

// TestStartSpinner_DisabledIsNoop asserts that a disabled spinner writes nothing and its stop func is idempotent.
func TestStartSpinner_DisabledIsNoop(t *testing.T) {
	var buf bytes.Buffer
	stop := StartSpinner(&buf, false, "working")
	stop()
	stop() // idempotent
	if buf.Len() != 0 {
		t.Fatalf("disabled spinner wrote %q, want nothing", buf.String())
	}
}

// TestStartSpinner_NilWriterIsNoop asserts that a nil writer is a no-op and stopping does not panic.
func TestStartSpinner_NilWriterIsNoop(t *testing.T) {
	stop := StartSpinner(nil, true, "working")
	stop() // must not panic
}

// TestStartSpinner_EnabledAnimatesAndCleansUp asserts that an enabled spinner renders a frame and label, then clears the line and restores the cursor on stop.
func TestStartSpinner_EnabledAnimatesAndCleansUp(t *testing.T) {
	var buf bytes.Buffer
	stop := StartSpinner(&buf, true, "Publishing")
	// The goroutine renders the first frame synchronously before selecting on
	// the stop channel, so even an immediate stop() yields one full cycle.
	stop()
	stop() // idempotent, must not panic or double-write after finished

	out := buf.String()
	if !strings.Contains(out, spinnerHideCursor) {
		t.Errorf("missing hide-cursor escape:\n%q", out)
	}
	if !strings.Contains(out, spinnerFrames[0]) {
		t.Errorf("missing first spinner frame %q:\n%q", spinnerFrames[0], out)
	}
	if !strings.Contains(out, "Publishing...") {
		t.Errorf("missing label:\n%q", out)
	}
	if !strings.Contains(out, spinnerClearLine) {
		t.Errorf("missing clear-line escape:\n%q", out)
	}
	if !strings.HasSuffix(out, spinnerShowCursor) {
		t.Errorf("must end by restoring the cursor:\n%q", out)
	}
}
