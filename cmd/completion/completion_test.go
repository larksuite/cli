// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package completion

import (
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

func TestNewCmdCompletion_IsVisibleAndAuthFree(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	cmd := NewCmdCompletion(f)
	if cmd.Hidden {
		t.Fatal("expected completion command to be visible")
	}
	if !cmdutil.IsAuthCheckDisabled(cmd) {
		t.Fatal("expected completion command to skip auth checks")
	}
}

