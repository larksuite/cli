// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lingo

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestEntityMatchValidate_MissingWord(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityMatch, f, stdout, []string{"+match"})
	if err == nil {
		t.Fatal("expected error for missing --word")
	}
	if !strings.Contains(err.Error(), "word") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntityMatchValidate_ControlCharsInWord(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityMatch, f, stdout, []string{
		"+match",
		"--word", "KYC\t",
	})
	if err == nil {
		t.Fatal("expected error for control chars in --word")
	}
}

func TestEntityMatchDryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityMatch, f, stdout, []string{
		"+match",
		"--word", "KYC",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/lingo/v1/entities/match") {
		t.Fatalf("dry-run output missing API path, got: %s", out)
	}
	if !strings.Contains(out, "KYC") {
		t.Fatalf("dry-run output missing word, got: %s", out)
	}
}

func TestEntityMatchExecute_NotFound(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/lingo/v1/entities/match",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"results": []interface{}{},
			},
		},
	})
	err := runLingoShortcut(t, LingoEntityMatch, f, stdout, []string{
		"+match",
		"--word", "nope",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	results, _ := data["results"].([]interface{})
	if len(results) != 0 {
		t.Fatalf("results = %v, want empty", results)
	}
}

func TestEntityMatchExecute_Found(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/lingo/v1/entities/match",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"results": []interface{}{
					map[string]interface{}{"entity_id": "ent-42", "type": 1},
				},
			},
		},
	})
	err := runLingoShortcut(t, LingoEntityMatch, f, stdout, []string{
		"+match",
		"--word", "KYC",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	results, _ := data["results"].([]interface{})
	if len(results) != 1 {
		t.Fatalf("results count = %d, want 1", len(results))
	}
}
