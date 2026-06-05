// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package hook

import "io"

// SetStderrForTesting redirects the hook layer's warning output to a
// custom writer and returns a restore function the caller MUST defer
// (or pass to `t.Cleanup`). Without the restore step, a later test in
// the same binary would inherit the override and either race on a
// shared bytes.Buffer or write user-visible garbage into a real test
// stderr.
//
// Production code never calls this; the default writer is os.Stderr
// via defaultStderr.
func SetStderrForTesting(w io.Writer) (restore func()) {
	prev := stderr
	stderr = func() interface{ Write(p []byte) (int, error) } {
		return w
	}
	return func() { stderr = prev }
}
