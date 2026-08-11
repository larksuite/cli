// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package shortcuts

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

func TestPrepareExternalRegistrationCopiesInput(t *testing.T) {
	commands := []common.Shortcut{{
		Service: "im", Command: "+external-copy", Scopes: []string{"im:chat:read"},
		Flags: []common.Flag{{Name: "id", Enum: []string{"one"}}},
	}}
	registered, err := prepareExternalRegistration(nil, commands, false)
	if err != nil {
		t.Fatal(err)
	}
	commands[0].Scopes[0] = "mutated"
	commands[0].Flags[0].Enum[0] = "mutated"
	if got := registered[0].Scopes[0]; got != "im:chat:read" {
		t.Fatalf("registered scope = %q", got)
	}
	if got := registered[0].Flags[0].Enum[0]; got != "one" {
		t.Fatalf("registered enum = %q", got)
	}
}

func TestPrepareExternalRegistrationRejectsWholeContribution(t *testing.T) {
	existing := []common.Shortcut{{Service: "im", Command: "+existing"}}
	commands := []common.Shortcut{
		{Service: "im", Command: "+new"},
		{Service: "im", Command: "+existing"},
	}
	registered, err := prepareExternalRegistration(existing, commands, false)
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("registration error = %v", err)
	}
	if registered != nil {
		t.Fatalf("registered commands = %#v", registered)
	}
}

func TestPrepareExternalRegistrationRejectsSecondContribution(t *testing.T) {
	registered, err := prepareExternalRegistration(nil, []common.Shortcut{{Service: "im", Command: "+second"}}, true)
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("registration error = %v", err)
	}
	if registered != nil {
		t.Fatalf("registered commands = %#v", registered)
	}
}
