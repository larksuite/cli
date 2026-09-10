// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/cmd"
	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/registry"
)

// TestNativeAffordanceExamplesMatchCommands checks that Examples already
// written for native methods use the current command path and accepted flags.
// It does not validate request values or require every method to be documented.
func TestNativeAffordanceExamplesMatchCommands(t *testing.T) {
	if os.Getenv("LARKSUITE_CLI_CHECK_AFFORDANCE_CONSISTENCY") != "1" {
		t.Skip("enabled by the affordance-document CI gate")
	}
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "off")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	methods := registry.EmbeddedCatalog().WalkMethods(nil)
	if len(methods) == 0 {
		t.Skip("embedded API metadata is unavailable in the bare-module registry")
	}
	cmd.SetEmbeddedAffordanceContent(os.DirFS("../affordance"))
	t.Cleanup(func() { cmd.SetEmbeddedAffordanceContent(nil) })

	cat := buildCmdExampleCatalog()
	for _, method := range methods {
		service, methodID := method.ServiceName(), method.Method.ID
		if strings.HasPrefix(methodID, "+") {
			continue
		}
		raw, ok := affordance.For(service, methodID)
		if !ok {
			continue
		}
		a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
		if !ok {
			t.Errorf("affordance %s/%s cannot be parsed", service, methodID)
			continue
		}
		for _, example := range a.Examples {
			exampleRef, ok := parseNativeAffordanceExample(example.Command)
			if !ok {
				t.Errorf("affordance %s/%s example must contain one lark-cli command:\n%s", service, methodID, example.Command)
				continue
			}
			if want := method.CommandPath(); !slices.Equal(exampleRef.words, want) {
				t.Errorf("affordance %s/%s example uses command %q, want %q:\n%s", service, methodID, strings.Join(exampleRef.words, " "), strings.Join(want, " "), example.Command)
				continue
			}
			for _, finding := range checkRefs(cat, []ref{exampleRef}) {
				reportAffordanceExampleFinding(t, service, methodID, example.Command, finding)
			}
		}
	}
}

// parseNativeAffordanceExample recognizes the documented convention: one
// direct lark-cli invocation followed by flags. It intentionally does not
// interpret shell syntax or validate flag values.
func parseNativeAffordanceExample(command string) (ref, bool) {
	tokens := strings.Fields(command)
	if len(tokens) == 0 || tokens[0] != cliToken {
		return ref{}, false
	}
	r := ref{raw: command}
	inFlags := false
	for _, token := range tokens[1:] {
		if strings.HasPrefix(token, "-") && token != "-" {
			inFlags = true
			r.flags = append(r.flags, strings.SplitN(token, "=", 2)[0])
			continue
		}
		if !inFlags {
			r.words = append(r.words, token)
		}
	}
	return r, len(r.words) > 0
}

func reportAffordanceExampleFinding(t *testing.T, service, methodID, example string, f finding) {
	t.Helper()
	hint := ""
	if f.suggest != "" {
		hint = " (did you mean " + f.suggest + "?)"
	}
	if f.kind == unknownFlag {
		t.Errorf("affordance %s/%s example uses unknown flag %s on %q%s:\n%s", service, methodID, f.flag, f.path, hint, example)
		return
	}
	t.Errorf("affordance %s/%s example uses unknown command %q%s:\n%s", service, methodID, f.path, hint, example)
}
