// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// testConfig returns a CliConfig wired with a stable user identity. Tests
// keep the AppID test-prefixed so logs / metrics can spot them.
func testConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	replacer := strings.NewReplacer("/", "-", " ", "-")
	suffix := replacer.Replace(strings.ToLower(t.Name()))
	return &core.CliConfig{
		AppID:      "test-sheets-" + suffix,
		AppSecret:  "secret-sheets-" + suffix,
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test_user",
	}
}

// newTestRig spins up a Factory wired with httpmock + the given shortcut
// mounted into a "sheets" parent command. Returns the cobra.Command ready
// to SetArgs / Execute, plus the stdout / stderr buffers and the registry.
func newTestRig(t *testing.T, sc common.Shortcut) (*cobra.Command, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	f, stdout, stderr, reg := cmdutil.TestFactory(t, testConfig(t))
	parent := &cobra.Command{Use: "sheets"}
	sc.Mount(parent, f)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	return parent, stdout, stderr, reg
}

// runShortcut executes the shortcut with the given args and returns the
// captured stdout text. Mirrors the legacy package's parent.Execute()
// flow so test cases stay close to real CLI behavior.
func runShortcut(t *testing.T, sc common.Shortcut, args []string) (string, error) {
	t.Helper()
	parent, stdout, _, _ := newTestRig(t, sc)
	parent.SetArgs(append([]string{sc.Command}, args...))
	err := parent.Execute()
	return stdout.String(), err
}

// runShortcutCapturingErr is runShortcut but also returns the stderr text
// so validation tests can inspect error envelopes.
func runShortcutCapturingErr(t *testing.T, sc common.Shortcut, args []string) (stdoutStr, stderrStr string, err error) {
	t.Helper()
	parent, stdout, stderr, _ := newTestRig(t, sc)
	parent.SetArgs(append([]string{sc.Command}, args...))
	err = parent.Execute()
	return stdout.String(), stderr.String(), err
}

// runShortcutWithStubs is runShortcut + a slice of httpmock stubs.
// Stubs are registered before execute so the recorded API calls are
// served from the registry instead of touching the network.
func runShortcutWithStubs(t *testing.T, sc common.Shortcut, args []string, stubs ...*httpmock.Stub) (string, error) {
	t.Helper()
	parent, stdout, _, reg := newTestRig(t, sc)
	for _, s := range stubs {
		reg.Register(s)
	}
	parent.SetArgs(append([]string{sc.Command}, args...))
	err := parent.Execute()
	return stdout.String(), err
}

// requireProblem asserts err carries a typed errs.Problem with the given
// category and (optional) subtype, and that its message contains msgContains
// (skip the message check by passing ""). Returns the Problem so callers can
// drill into the typed envelope's category-specific fields (e.g. cast to
// *errs.ValidationError to read .Param / .Params / .Cause).
//
// Replaces the older "strings.Contains(stdout+stderr+err.Error(), ...)" pattern
// across sheets tests: substring on a rendered envelope was brittle (any
// message tweak silently broke it) and didn't verify that the typed contract —
// category / subtype / cause preservation — held. Per coding guideline
// "Error-path tests must assert typed metadata via errs.ProblemOf
// (category / subtype / param) and cause preservation, not message substrings
// alone."
func requireProblem(t *testing.T, err error, wantCategory errs.Category, wantSubtype errs.Subtype, msgContains string) *errs.Problem {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error carrying errs.Problem, got %T: %v", err, err)
	}
	if p.Category != wantCategory {
		t.Errorf("category = %q, want %q (err=%v)", p.Category, wantCategory, err)
	}
	if wantSubtype != "" && p.Subtype != wantSubtype {
		t.Errorf("subtype = %q, want %q (err=%v)", p.Subtype, wantSubtype, err)
	}
	if msgContains != "" && !strings.Contains(p.Message, msgContains) {
		t.Errorf("message = %q, want containing %q", p.Message, msgContains)
	}
	return p
}

// requireValidation is shorthand for the most common case: a typed
// CategoryValidation error with SubtypeInvalidArgument. Returns the
// *errs.ValidationError so callers can also assert on .Param / .Params / .Cause.
func requireValidation(t *testing.T, err error, msgContains string) *errs.ValidationError {
	t.Helper()
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, msgContains)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	return ve
}

func TestSheetHelpersValidationMetadata(t *testing.T) {
	t.Parallel()

	t.Run("missing sheet selector reports both params", func(t *testing.T) {
		t.Parallel()
		err := requireSheetSelector("", "")
		var validationErr *errs.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
		}
		if len(validationErr.Params) != 2 {
			t.Fatalf("params = %#v, want two structured params", validationErr.Params)
		}
		if validationErr.Params[0].Name != "--sheet-id" || validationErr.Params[1].Name != "--sheet-name" {
			t.Fatalf("params = %#v, want --sheet-id/--sheet-name", validationErr.Params)
		}
	})

	t.Run("spreadsheet url shape reports url param", func(t *testing.T) {
		t.Parallel()
		cmd := &cobra.Command{Use: "sheets"}
		cmd.Flags().String("url", "not-a-sheet-url", "")
		cmd.Flags().String("spreadsheet-token", "", "")
		_, err := resolveSpreadsheetToken(common.TestNewRuntimeContext(cmd, testConfig(t)))
		var validationErr *errs.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
		}
		if validationErr.Param != "--url" {
			t.Fatalf("param = %q, want --url", validationErr.Param)
		}
	})

	t.Run("sheet selector control char keeps param and cause", func(t *testing.T) {
		t.Parallel()
		err := requireSheetSelector("bad\x00id", "")
		var validationErr *errs.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
		}
		if validationErr.Param != "--sheet-id" {
			t.Fatalf("param = %q, want --sheet-id", validationErr.Param)
		}
		if validationErr.Unwrap() == nil {
			t.Fatalf("expected control-char validation cause to be preserved")
		}
	})

	t.Run("invalid json flag keeps param and cause", func(t *testing.T) {
		t.Parallel()
		fv := newMapFlagViewForCommand("+cells-set", map[string]interface{}{"cells": "{"})
		_, err := parseJSONFlag(fv, "cells")
		var validationErr *errs.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
		}
		if validationErr.Param != "--cells" {
			t.Fatalf("param = %q, want --cells", validationErr.Param)
		}
		if validationErr.Unwrap() == nil {
			t.Fatalf("expected JSON parse cause to be preserved")
		}
	})
}

// parseDryRunBody runs the shortcut in --dry-run and returns the first
// api call's body. The dry-run output format is:
//
//	=== Dry Run ===
//	{ "api": [{...}], ... }
//
// Tests use this to assert the One-OpenAPI wire body is constructed
// correctly without exercising the real endpoint.
func parseDryRunBody(t *testing.T, sc common.Shortcut, args []string) map[string]interface{} {
	t.Helper()
	out, err := runShortcut(t, sc, append(args, "--dry-run"))
	if err != nil {
		t.Fatalf("dry-run failed: %v\noutput=%s", err, out)
	}
	return decodeDryRunFirstCall(t, out)
}

// parseDryRunAPI returns the full list of `api` entries from a dry-run
// output — used by shortcuts that emit multiple calls (e.g.
// +workbook-export, +cells-set-image, +cells-batch-set-style).
func parseDryRunAPI(t *testing.T, sc common.Shortcut, args []string) []interface{} {
	t.Helper()
	out, err := runShortcut(t, sc, append(args, "--dry-run"))
	if err != nil {
		t.Fatalf("dry-run failed: %v\noutput=%s", err, out)
	}
	dryRun := decodeDryRunRaw(t, out)
	calls, _ := dryRun["api"].([]interface{})
	return calls
}

func decodeDryRunRaw(t *testing.T, out string) map[string]interface{} {
	t.Helper()
	idx := strings.Index(out, "{")
	if idx < 0 {
		t.Fatalf("dry-run output has no JSON body:\n%s", out)
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(out[idx:]), &m); err != nil {
		t.Fatalf("failed to parse dry-run JSON: %v\nraw=%s", err, out)
	}
	return m
}

func decodeDryRunFirstCall(t *testing.T, out string) map[string]interface{} {
	t.Helper()
	dryRun := decodeDryRunRaw(t, out)
	calls, ok := dryRun["api"].([]interface{})
	if !ok || len(calls) == 0 {
		t.Fatalf("dry-run api array empty or wrong shape: %#v", dryRun)
	}
	call, _ := calls[0].(map[string]interface{})
	body, _ := call["body"].(map[string]interface{})
	if body == nil {
		t.Fatalf("dry-run first call has no body: %#v", call)
	}
	return body
}

// decodeToolInput parses the JSON-string `input` field embedded in a
// dry-run body whose tool_name matches `expected`. Returns the decoded
// tool input map so tests can assert on specific input fields.
func decodeToolInput(t *testing.T, body map[string]interface{}, expectedToolName string) map[string]interface{} {
	t.Helper()
	if got, _ := body["tool_name"].(string); got != expectedToolName {
		t.Fatalf("tool_name = %q, want %q", got, expectedToolName)
	}
	rawInput, _ := body["input"].(string)
	if rawInput == "" {
		t.Fatalf("body.input is empty: %#v", body)
	}
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(rawInput), &input); err != nil {
		t.Fatalf("failed to parse tool input JSON: %v\nraw=%s", err, rawInput)
	}
	return input
}

// decodeEnvelopeData parses a successful envelope's data field — used by
// execute-path tests that go through the full callTool stack with stubs.
func decodeEnvelopeData(t *testing.T, out string) map[string]interface{} {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to decode envelope: %v\nraw=%s", err, out)
	}
	if ok, _ := envelope["ok"].(bool); !ok {
		t.Fatalf("envelope.ok=false: %#v", envelope)
	}
	data, _ := envelope["data"].(map[string]interface{})
	return data
}

// toolOutputStub builds an httpmock stub for the One-OpenAPI invoke_read
// or invoke_write endpoint. `outputJSON` is the JSON string the tool
// returns in data.output.
func toolOutputStub(token, kind string, outputJSON string) *httpmock.Stub {
	suffix := "invoke_read"
	if kind == "write" {
		suffix = "invoke_write"
	}
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheet_ai/v2/spreadsheets/" + token + "/tools/" + suffix,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"output": outputJSON,
			},
		},
	}
}

// commonArgsURL is the typical --url and --sheet-id pair used by sheet-
// level tests.
const (
	testToken    = "shtcnTestTOK"
	testURL      = "https://example.feishu.cn/sheets/shtcnTestTOK"
	testSheetID  = "shtSubA"
	testSheetID2 = "shtSubB"
)

// TestParseSpreadsheetRef locks the network-free classification of
// --url / --spreadsheet-token into a sheet token vs an (unresolved) wiki
// node_token. The wiki node is resolved later, at Execute time only.
func TestParseSpreadsheetRef(t *testing.T) {
	t.Parallel()
	mk := func(url, tok string) *common.RuntimeContext {
		cmd := &cobra.Command{Use: "sheets"}
		cmd.Flags().String("url", url, "")
		cmd.Flags().String("spreadsheet-token", tok, "")
		return common.TestNewRuntimeContext(cmd, testConfig(t))
	}
	cases := []struct {
		name      string
		url       string
		tok       string
		wantKind  string
		wantToken string
		wantErr   bool
	}{
		{name: "sheets url", url: "https://x.feishu.cn/sheets/shtABC", wantKind: spreadsheetRefSheet, wantToken: "shtABC"},
		{name: "spreadsheets url", url: "https://x.feishu.cn/spreadsheets/shtABC", wantKind: spreadsheetRefSheet, wantToken: "shtABC"},
		{name: "wiki url", url: "https://x.feishu.cn/wiki/wikDEF", wantKind: spreadsheetRefWiki, wantToken: "wikDEF"},
		{name: "wiki url with query", url: "https://x.feishu.cn/wiki/wikDEF?sheet=xxxxxx", wantKind: spreadsheetRefWiki, wantToken: "wikDEF"},
		{name: "raw token", tok: "shtRAW", wantKind: spreadsheetRefSheet, wantToken: "shtRAW"},
		{name: "sheets url with /wiki/ in query stays sheet", url: "https://x.feishu.cn/sheets/shtABC?from=/wiki/wikX", wantKind: spreadsheetRefSheet, wantToken: "shtABC"},
		{name: "sheets url with /wiki/ in fragment stays sheet", url: "https://x.feishu.cn/sheets/shtABC#/wiki/wikX", wantKind: spreadsheetRefSheet, wantToken: "shtABC"},
		{name: "docx url unsupported", url: "https://x.feishu.cn/docx/docABC", wantErr: true},
		{name: "neither provided", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ref, err := parseSpreadsheetRef(mk(tc.url, tc.tok))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got ref=%+v", ref)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ref.Kind != tc.wantKind || ref.Token != tc.wantToken {
				t.Fatalf("ref = %+v, want {Kind:%s Token:%s}", ref, tc.wantKind, tc.wantToken)
			}
		})
	}
}
