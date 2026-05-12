// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lingo

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestEntitySearchValidate_MissingQuery(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntitySearch, f, stdout, []string{"+search"})
	if err == nil {
		t.Fatal("expected error for missing --query")
	}
	if !strings.Contains(err.Error(), "query") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntitySearchValidate_PageSizeOutOfRange(t *testing.T) {
	t.Parallel()
	cases := []string{"0", "101", "-5"}
	for _, size := range cases {
		f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
		err := runLingoShortcut(t, LingoEntitySearch, f, stdout, []string{
			"+search",
			"--query", "KYC",
			"--page-size", size,
		})
		if err == nil {
			t.Fatalf("page-size=%s: expected error", size)
		}
		if !strings.Contains(err.Error(), "page-size") {
			t.Fatalf("page-size=%s: unexpected error: %v", size, err)
		}
	}
}

func TestEntitySearchValidate_ControlCharsInQuery(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntitySearch, f, stdout, []string{
		"+search",
		"--query", "KYC\t01",
	})
	if err == nil {
		t.Fatal("expected error for control chars in --query")
	}
}

func TestEntitySearchDryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntitySearch, f, stdout, []string{
		"+search",
		"--query", "飞书",
		"--page-size", "30",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/lingo/v1/entities/search") {
		t.Fatalf("dry-run output missing API path, got: %s", out)
	}
	if !strings.Contains(out, "飞书") {
		t.Fatalf("dry-run output missing query, got: %s", out)
	}
}

func TestEntitySearchExecute_NoMatches(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/lingo/v1/entities/search",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"entities": []interface{}{},
			},
		},
	})
	err := runLingoShortcut(t, LingoEntitySearch, f, stdout, []string{
		"+search",
		"--query", "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	entities, _ := data["entities"].([]interface{})
	if len(entities) != 0 {
		t.Fatalf("entities = %v, want empty", entities)
	}
}

func TestEntitySearchExecute_WithMatches(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/lingo/v1/entities/search",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"entities": []interface{}{
					map[string]interface{}{
						"id":          "ent-1",
						"main_keys":   []interface{}{map[string]interface{}{"key": "飞书"}},
						"description": "企业协作平台",
					},
				},
			},
		},
	})
	err := runLingoShortcut(t, LingoEntitySearch, f, stdout, []string{
		"+search",
		"--query", "飞书",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	entities, _ := data["entities"].([]interface{})
	if len(entities) != 1 {
		t.Fatalf("entities count = %d, want 1", len(entities))
	}
}

func TestEntitySearchExecute_APIError(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/lingo/v1/entities/search",
		Status: 500,
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "internal error",
		},
	})
	err := runLingoShortcut(t, LingoEntitySearch, f, stdout, []string{
		"+search",
		"--query", "x",
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}
