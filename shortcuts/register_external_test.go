// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package shortcuts

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

func TestAllShortcutsWithExternalCopiesInput(t *testing.T) {
	commands := []common.Shortcut{{
		Service: "external-fixture", Command: "+external-copy", Scopes: []string{"im:chat:read"},
		Flags: []common.Flag{{Name: "id", Enum: []string{"one"}}},
	}}
	registered, err := AllShortcutsWithExternal(commands)
	if err != nil {
		t.Fatal(err)
	}
	external := registered[len(registered)-1]
	commands[0].Scopes[0] = "mutated"
	commands[0].Flags[0].Enum[0] = "mutated"
	if got := external.Scopes[0]; got != "im:chat:read" {
		t.Fatalf("registered scope = %q", got)
	}
	if got := external.Flags[0].Enum[0]; got != "one" {
		t.Fatalf("registered enum = %q", got)
	}
}

func TestAllShortcutsWithExternalRejectsWholeContribution(t *testing.T) {
	existing := AllShortcuts()[0]
	commands := []common.Shortcut{
		{Service: "external-fixture", Command: "+new"},
		{Service: existing.Service, Command: existing.Command},
	}
	registered, err := AllShortcutsWithExternal(commands)
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("registration error = %v", err)
	}
	if registered != nil {
		t.Fatalf("registered commands = %#v", registered)
	}
}

func TestAllShortcutsWithExternalRejectsDuplicateExternalPath(t *testing.T) {
	commands := []common.Shortcut{
		{Service: "external-fixture", Command: "+duplicate"},
		{Service: "external-fixture", Command: "+duplicate"},
	}
	registered, err := AllShortcutsWithExternal(commands)
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("registration error = %v", err)
	}
	if registered != nil {
		t.Fatalf("registered commands = %#v", registered)
	}
}
