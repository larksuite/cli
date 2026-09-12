// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

// contentLikeFlags lists the text-content flags on +messages-send that should
// support reading from stdin (-) and files (@path).
var contentLikeFlags = []string{"content", "text", "markdown"}

func TestMessagesSend_ContentFlagsSupportFileAndStdin(t *testing.T) {
	flagMap := map[string]common.Flag{}
	for _, f := range ImMessagesSend.Flags {
		flagMap[f.Name] = f
	}

	for _, name := range contentLikeFlags {
		f, ok := flagMap[name]
		if !ok {
			t.Fatalf("flag --%s not found on +messages-send", name)
		}
		hasFile := false
		hasStdin := false
		for _, src := range f.Input {
			if src == common.File {
				hasFile = true
			}
			if src == common.Stdin {
				hasStdin = true
			}
		}
		if !hasFile {
			t.Errorf("flag --%s should declare Input: %q source", name, common.File)
		}
		if !hasStdin {
			t.Errorf("flag --%s should declare Input: %q source", name, common.Stdin)
		}
	}
}
