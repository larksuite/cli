// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/shortcuts/common"
)

var imAffordanceFlagToken = regexp.MustCompile(`--[a-z][a-z0-9-]*`)

var imFrameworkFlags = map[string]bool{
	"--as": true, "--dry-run": true, "--format": true, "--json": true, "--jq": true, "--yes": true,
}

func TestAllIMShortcutsUseAffordanceExamples(t *testing.T) {
	affordance.SetSource(os.DirFS("../../affordance"))
	t.Cleanup(func() { affordance.SetSource(nil) })

	shortcuts := Shortcuts()
	if got, want := len(shortcuts), 23; got != want {
		t.Fatalf("registered IM shortcuts = %d, want audited count %d", got, want)
	}

	for _, sc := range shortcuts {
		t.Run(sc.Command, func(t *testing.T) {
			for _, tip := range sc.Tips {
				if strings.Contains(tip, "lark-cli im ") {
					t.Fatalf("copyable example is still mixed into Go Tips: %q", tip)
				}
			}

			raw, ok := affordance.For("im", sc.Command)
			if !ok {
				t.Fatalf("missing affordance for registered shortcut %s", sc.Command)
			}
			parsed, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
			if !ok || len(parsed.Examples) != 1 {
				t.Fatalf("%s examples = %#v, want exactly one", sc.Command, parsed.Examples)
			}
			example := parsed.Examples[0].Command
			if !strings.HasPrefix(example, "lark-cli im "+sc.Command) {
				t.Fatalf("example does not invoke its shortcut: %q", example)
			}
			assertIMExampleFlagsExist(t, sc, example)
			assertIMExampleIdentity(t, sc, example)
			assertIMFirstExampleCoversRequiredFlags(t, sc, example)
		})
	}
}

func TestChatMembersTipsMovedToAffordance(t *testing.T) {
	if len(ImChatMembersList.Tips) != 0 {
		t.Fatalf("Go Tips must be empty after migration, got %v", ImChatMembersList.Tips)
	}

	affordance.SetSource(os.DirFS("../../affordance"))
	t.Cleanup(func() { affordance.SetSource(nil) })
	raw, ok := affordance.For("im", "+chat-members-list")
	if !ok {
		t.Fatal("missing +chat-members-list affordance")
	}
	parsed, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		t.Fatal("+chat-members-list affordance did not parse")
	}
	want := []string{
		"Default fetches a single page; pass --page-all to walk every page.",
		"With --page-all and no explicit --page-size, the max page size is used to minimize round-trips.",
		"truncations[] in the result means the server capped a bucket due to security config — the member list is incomplete.",
	}
	if strings.Join(parsed.Tips, "\n") != strings.Join(want, "\n") {
		t.Fatalf("migrated tips = %v, want %v", parsed.Tips, want)
	}
}

func assertIMExampleFlagsExist(t *testing.T, sc common.Shortcut, example string) {
	t.Helper()
	declared := map[string]bool{}
	for _, flag := range sc.Flags {
		declared["--"+flag.Name] = true
	}
	for _, token := range imAffordanceFlagToken.FindAllString(example, -1) {
		if !declared[token] && !imFrameworkFlags[token] {
			t.Errorf("example uses undeclared flag %s: %s", token, example)
		}
	}
}

func assertIMExampleIdentity(t *testing.T, sc common.Shortcut, example string) {
	t.Helper()
	allowed := map[string]bool{}
	for _, identity := range sc.AuthTypes {
		allowed[identity] = true
	}
	identity := explicitIdentity(example)
	if len(allowed) == 1 {
		if identity == "" {
			t.Errorf("single-identity shortcut example must pin its identity: %s", example)
			return
		}
		if !allowed[identity] {
			t.Errorf("example pins unsupported identity %q: %s", identity, example)
		}
		return
	}
	if allowed["user"] && allowed["bot"] && identity != "" {
		t.Errorf("dual-identity shortcut example must leave identity to user intent: %s", example)
	}
}

func assertIMFirstExampleCoversRequiredFlags(t *testing.T, sc common.Shortcut, example string) {
	t.Helper()
	tokens := map[string]bool{}
	for _, token := range imAffordanceFlagToken.FindAllString(example, -1) {
		tokens[token] = true
	}
	for _, flag := range sc.Flags {
		if flag.Required && !tokens["--"+flag.Name] {
			t.Errorf("first example does not cover required flag --%s: %s", flag.Name, example)
		}
	}
}

func explicitIdentity(example string) string {
	fields := strings.Fields(example)
	for i, field := range fields {
		if field == "--as" && i+1 < len(fields) {
			return fields[i+1]
		}
		if strings.HasPrefix(field, "--as=") {
			return strings.TrimPrefix(field, "--as=")
		}
	}
	return ""
}
