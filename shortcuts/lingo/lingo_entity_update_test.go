// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lingo

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestEntityUpdateValidate_MissingEntityID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityUpdate, f, stdout, []string{
		"+update",
		"--main-key", "KYC",
	})
	if err == nil {
		t.Fatal("expected error for missing --entity-id")
	}
	if !strings.Contains(err.Error(), "entity-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntityUpdateValidate_MissingMainKey(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityUpdate, f, stdout, []string{
		"+update",
		"--entity-id", "ent-1",
	})
	if err == nil {
		t.Fatal("expected error for missing --main-key")
	}
	if !strings.Contains(err.Error(), "main-key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntityUpdateDryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityUpdate, f, stdout, []string{
		"+update",
		"--entity-id", "ent-1",
		"--main-key", "KYC",
		"--description", "updated desc",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/lingo/v1/entities/ent-1") {
		t.Fatalf("dry-run output missing resolved path, got: %s", out)
	}
	if !strings.Contains(out, "PUT") {
		t.Fatalf("dry-run should be PUT, got: %s", out)
	}
	if !strings.Contains(out, "updated desc") {
		t.Fatalf("dry-run output missing description, got: %s", out)
	}
}

func TestEntityUpdateExecute_OK(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/lingo/v1/entities/ent-1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"entity": map[string]interface{}{
					"id":        "ent-1",
					"main_keys": []interface{}{map[string]interface{}{"key": "KYC"}},
				},
			},
		},
	})
	err := runLingoShortcut(t, LingoEntityUpdate, f, stdout, []string{
		"+update",
		"--entity-id", "ent-1",
		"--main-key", "KYC",
		"--description", "new",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	entity, _ := data["entity"].(map[string]interface{})
	if entity == nil || entity["id"] != "ent-1" {
		t.Fatalf("unexpected entity in data: %#v", data)
	}
}
