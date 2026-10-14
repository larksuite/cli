// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lingo

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestEntityGetValidate_MissingEntityID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityGet, f, stdout, []string{"+get"})
	if err == nil {
		t.Fatal("expected error for missing --entity-id")
	}
	if !strings.Contains(err.Error(), "entity-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntityGetValidate_ControlCharsInEntityID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityGet, f, stdout, []string{
		"+get",
		"--entity-id", "ent\t-1",
	})
	if err == nil {
		t.Fatal("expected error for control chars in --entity-id")
	}
}

func TestEntityGetDryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityGet, f, stdout, []string{
		"+get",
		"--entity-id", "ent-1",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/lingo/v1/entities/ent-1") {
		t.Fatalf("dry-run output missing resolved path, got: %s", out)
	}
}

func TestEntityGetDryRun_WithOuterInfo(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityGet, f, stdout, []string{
		"+get",
		"--entity-id", "ent-1",
		"--provider", "myhr",
		"--outer-id", "EMP-001",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "myhr") || !strings.Contains(out, "EMP-001") {
		t.Fatalf("dry-run output missing outer-info params, got: %s", out)
	}
}

func TestEntityGetExecute_OK(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/lingo/v1/entities/ent-1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"entity": map[string]interface{}{
					"id":          "ent-1",
					"main_keys":   []interface{}{map[string]interface{}{"key": "飞书"}},
					"aliases":     []interface{}{map[string]interface{}{"key": "Lark"}},
					"description": "企业协作平台",
				},
			},
		},
	})
	err := runLingoShortcut(t, LingoEntityGet, f, stdout, []string{
		"+get",
		"--entity-id", "ent-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	entity, _ := data["entity"].(map[string]interface{})
	if entity == nil {
		t.Fatal("missing entity in data")
	}
	if entity["id"] != "ent-1" {
		t.Fatalf("entity id = %v, want ent-1", entity["id"])
	}
}
