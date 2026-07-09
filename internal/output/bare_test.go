// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output_test

import (
	"testing"

	"github.com/larksuite/cli/internal/output"
)

func TestExitCodeOfBareError(t *testing.T) {
	if got := output.ExitCodeOf(output.ErrBare(3)); got != 3 {
		t.Errorf("ExitCodeOf(ErrBare(3)) = %d, want 3", got)
	}
}

// TestErrBareReturnsBareError pins that the silent-exit signal is the
// dedicated *output.BareError type, keeping that contract on its own
// narrow signal type.
func TestErrBareReturnsBareError(t *testing.T) {
	var _ *output.BareError = output.ErrBare(1)
}
