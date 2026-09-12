// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lingo

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestEntityDeleteValidate_MissingEntityID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityDelete, f, stdout, []string{"+delete", "--yes"})
	if err == nil {
		t.Fatal("expected error for missing --entity-id")
	}
	if !strings.Contains(err.Error(), "entity-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntityDeleteWithoutYes_Refused(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityDelete, f, stdout, []string{
		"+delete",
		"--entity-id", "ent-1",
	})
	if err == nil {
		t.Fatal("expected high-risk-write to refuse without --yes")
	}
}

func TestEntityDeleteDryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityDelete, f, stdout, []string{
		"+delete",
		"--entity-id", "ent-1",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "DELETE") {
		t.Fatalf("dry-run should be DELETE, got: %s", out)
	}
	if !strings.Contains(out, "/open-apis/lingo/v1/entities/ent-1") {
		t.Fatalf("dry-run output missing resolved path, got: %s", out)
	}
}

func TestEntityDeleteExecute_OK(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/lingo/v1/entities/ent-1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	})
	err := runLingoShortcut(t, LingoEntityDelete, f, stdout, []string{
		"+delete",
		"--entity-id", "ent-1",
		"--yes",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	if deleted, _ := data["deleted"].(bool); !deleted {
		t.Fatalf("expected deleted=true, got data=%#v", data)
	}
	if data["entity_id"] != "ent-1" {
		t.Fatalf("entity_id = %v, want ent-1", data["entity_id"])
	}
}
