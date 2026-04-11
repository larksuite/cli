// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package shortcuts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func TestAllShortcutsScopesNotNil(t *testing.T) {
	for _, s := range allShortcuts {
		hasScopes := s.Scopes != nil || s.UserScopes != nil || s.BotScopes != nil
		if !hasScopes {
			t.Errorf("shortcut %s/%s: Scopes is nil (must be explicitly set, use []string{} if no scopes needed)", s.Service, s.Command)
		}
	}
}

func TestAllShortcutsReturnsCopyAndIncludesBase(t *testing.T) {
	shortcuts := AllShortcuts()
	if len(shortcuts) == 0 {
		t.Fatal("AllShortcuts returned empty slice")
	}

	hasBaseGet := false
	for _, shortcut := range shortcuts {
		if shortcut.Service == "base" && shortcut.Command == "+base-get" {
			hasBaseGet = true
			break
		}
	}
	if !hasBaseGet {
		t.Fatal("AllShortcuts does not include base/+base-get")
	}

	shortcuts[0].Service = "mutated"
	if AllShortcuts()[0].Service == "mutated" {
		t.Fatal("AllShortcuts should return a copy")
	}
}

func TestRegisterShortcutsMountsBaseCommands(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, &cmdutil.Factory{})

	baseCmd, _, err := program.Find([]string{"base"})
	if err != nil {
		t.Fatalf("find base command: %v", err)
	}
	if baseCmd == nil || baseCmd.Name() != "base" {
		t.Fatalf("base command not mounted: %#v", baseCmd)
	}

	workspaceCmd, _, err := program.Find([]string{"base", "+base-get"})
	if err != nil {
		t.Fatalf("find base workspace shortcut: %v", err)
	}
	if workspaceCmd == nil || workspaceCmd.Name() != "+base-get" {
		t.Fatalf("base workspace shortcut not mounted: %#v", workspaceCmd)
	}
}

func TestRegisterShortcutsMountsDocsMediaPreview(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, &cmdutil.Factory{})

	previewCmd, _, err := program.Find([]string{"docs", "+media-preview"})
	if err != nil {
		t.Fatalf("find docs media preview shortcut: %v", err)
	}
	if previewCmd == nil || previewCmd.Name() != "+media-preview" {
		t.Fatalf("docs media preview shortcut not mounted: %#v", previewCmd)
	}
}

func TestRegisterShortcutsReusesExistingServiceCommand(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	existingBase := &cobra.Command{Use: "base", Short: "existing base service"}
	program.AddCommand(existingBase)

	RegisterShortcuts(program, &cmdutil.Factory{})

	baseCount := 0
	for _, command := range program.Commands() {
		if command.Name() == "base" {
			baseCount++
		}
	}
	if baseCount != 1 {
		t.Fatalf("expected 1 base service command, got %d", baseCount)
	}

	workspaceCmd, _, err := program.Find([]string{"base", "+base-get"})
	if err != nil {
		t.Fatalf("find base workspace shortcut under existing service: %v", err)
	}
	if workspaceCmd == nil {
		t.Fatal("base workspace shortcut not mounted on existing service command")
	}
}

func TestGenerateShortcutsJSON(t *testing.T) {
	output := os.Getenv("SHORTCUTS_OUTPUT")
	if output == "" {
		t.Skip("set SHORTCUTS_OUTPUT env to generate shortcuts.json")
	}

	shortcuts := AllShortcuts()

	type entry struct {
		Verb        string   `json:"verb"`
		Description string   `json:"description"`
		Scopes      []string `json:"scopes"`
	}
	grouped := make(map[string][]entry)
	for _, s := range shortcuts {
		verb := strings.TrimPrefix(s.Command, "+")
		grouped[s.Service] = append(grouped[s.Service], entry{
			Verb:        verb,
			Description: s.Description,
			Scopes:      s.ScopesForIdentity("user"),
		})
	}

	data, err := json.MarshalIndent(grouped, "", "  ")
	if err != nil {
		t.Fatalf("marshal shortcuts: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(output, data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	t.Logf("wrote %d bytes to %s", len(data), output)
}
