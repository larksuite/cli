// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"slices"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

// TestMessagesSendContentFlagsAcceptFileAndStdin pins that the content-like
// flags (--content / --text / --markdown) declare @file and stdin input
// sources. These carry large JSON / Markdown payloads whose shell quoting is
// error-prone on Windows, so callers must be able to pipe them via @file / -.
func TestMessagesSendContentFlagsAcceptFileAndStdin(t *testing.T) {
	byName := map[string]common.Flag{}
	for _, f := range ImMessagesSend.Flags {
		byName[f.Name] = f
	}

	for _, name := range []string{"content", "text", "markdown"} {
		f, ok := byName[name]
		if !ok {
			t.Fatalf("--%s flag not found", name)
		}
		for _, src := range []string{common.File, common.Stdin} {
			if !slices.Contains(f.Input, src) {
				t.Errorf("--%s should declare input source %q, got %v", name, src, f.Input)
			}
		}
	}
}
