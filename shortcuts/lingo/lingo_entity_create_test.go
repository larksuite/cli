// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lingo

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestEntityCreateValidate_MissingMainKey(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityCreate, f, stdout, []string{"+create"})
	if err == nil {
		t.Fatal("expected error for missing --main-key")
	}
	if !strings.Contains(err.Error(), "main-key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntityCreateValidate_BlankMainKey(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityCreate, f, stdout, []string{
		"+create",
		"--main-key", "   ",
	})
	if err == nil {
		t.Fatal("expected error for whitespace-only --main-key")
	}
	if !strings.Contains(err.Error(), "main-key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntityCreateDryRun_MinimalBody(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityCreate, f, stdout, []string{
		"+create",
		"--main-key", "KYC",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/lingo/v1/entities") {
		t.Fatalf("dry-run output missing API path, got: %s", out)
	}
	if !strings.Contains(out, "KYC") {
		t.Fatalf("dry-run output missing main-key, got: %s", out)
	}
	if !strings.Contains(out, "main_keys") {
		t.Fatalf("dry-run body should contain main_keys, got: %s", out)
	}
	// No aliases / description provided → those keys must be absent.
	if strings.Contains(out, "\"aliases\"") {
		t.Fatalf("dry-run body should NOT contain aliases when flag absent, got: %s", out)
	}
	if strings.Contains(out, "\"description\"") {
		t.Fatalf("dry-run body should NOT contain description when flag absent, got: %s", out)
	}
}

func TestEntityCreateDryRun_AliasesAndDescription(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityCreate, f, stdout, []string{
		"+create",
		"--main-key", "飞书",
		"--aliases", "Lark, FeiShu , 飞书办公",
		"--description", "企业协作平台",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"Lark", "FeiShu", "飞书办公", "企业协作平台"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run missing %q, got: %s", want, out)
		}
	}
}

func TestEntityCreateExecute_OK(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/lingo/v1/entities",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"entity": map[string]interface{}{
					"id":        "ent-new",
					"main_keys": []interface{}{map[string]interface{}{"key": "KYC"}},
				},
			},
		},
	})
	err := runLingoShortcut(t, LingoEntityCreate, f, stdout, []string{
		"+create",
		"--main-key", "KYC",
		"--description", "Know Your Customer",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	entity, _ := data["entity"].(map[string]interface{})
	if entity == nil || entity["id"] != "ent-new" {
		t.Fatalf("missing or wrong entity in data: %#v", data)
	}
}

func TestEntityCreateExecute_PermissionError(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/lingo/v1/entities",
		Body: map[string]interface{}{
			"code": 99991672,
			"msg":  "Permission denied",
		},
	})
	err := runLingoShortcut(t, LingoEntityCreate, f, stdout, []string{
		"+create",
		"--main-key", "KYC",
	})
	if err == nil {
		t.Fatal("expected error for API permission failure")
	}
}
