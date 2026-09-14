// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	baseshortcuts "github.com/larksuite/cli/shortcuts/base"
)

// Command help must not route agents to references omitted from the binary.
func TestBaseHelpReferencesShip(t *testing.T) {
	reference := regexp.MustCompile(`lark-base-[a-z0-9-]+\.md`)
	for _, shortcut := range baseshortcuts.Shortcuts() {
		t.Run(shortcut.Command, func(t *testing.T) {
			guidance := append([]string{shortcut.Description}, shortcut.Tips...)
			for _, flag := range shortcut.Flags {
				guidance = append(guidance, flag.Desc)
			}
			for _, name := range reference.FindAllString(strings.Join(guidance, "\n"), -1) {
				if _, err := fs.ReadFile(embeddedContentFS, "skills/lark-base/references/"+name); err != nil {
					t.Errorf("help references unavailable guide %s: %v", name, err)
				}
			}
		})
	}
}
