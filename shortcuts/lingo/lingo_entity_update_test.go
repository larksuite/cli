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

// --- rich_text tests ---

func TestEntityUpdateValidate_DescriptionAndRichTextMutuallyExclusive(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityUpdate, f, stdout, []string{
		"+update",
		"--entity-id", "ent-1",
		"--main-key", "KYC",
		"--description", "plain",
		"--rich-text", "<p>html</p>",
	})
	if err == nil {
		t.Fatal("expected error for passing both --description and --rich-text")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntityUpdateDryRun_RichText(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityUpdate, f, stdout, []string{
		"+update",
		"--entity-id", "ent-1",
		"--main-key", "KYC",
		"--rich-text", "<p><b>KYC</b><span>updated</span></p>",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "rich_text") {
		t.Fatalf("dry-run body should contain rich_text, got: %s", out)
	}
	if strings.Contains(out, "\"description\"") {
		t.Fatalf("dry-run body should NOT contain description when only --rich-text provided, got: %s", out)
	}
}

func TestEntityUpdateExecute_RichText(t *testing.T) {
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
					"rich_text": "<p><b>KYC</b></p>",
				},
			},
		},
	})
	err := runLingoShortcut(t, LingoEntityUpdate, f, stdout, []string{
		"+update",
		"--entity-id", "ent-1",
		"--main-key", "KYC",
		"--rich-text", "<p><b>KYC</b></p>",
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

// --- related_meta tests ---

func TestEntityUpdateValidate_InvalidRelatedMeta(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	err := runLingoShortcut(t, LingoEntityUpdate, f, stdout, []string{
		"+update",
		"--entity-id", "ent-1",
		"--main-key", "KYC",
		"--related-meta", "not-json",
	})
	if err == nil {
		t.Fatal("expected error for invalid --related-meta")
	}
	if !strings.Contains(err.Error(), "related-meta") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntityUpdateDryRun_RelatedMeta(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
	rm := `{"users":[{"id":"ou_xxx","title":"人名"}],"docs":[{"title":"KYC 流程","url":"https://feishu.cn/docs/yyy"}]}`
	err := runLingoShortcut(t, LingoEntityUpdate, f, stdout, []string{
		"+update",
		"--entity-id", "ent-1",
		"--main-key", "KYC",
		"--related-meta", rm,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "related_meta") {
		t.Fatalf("dry-run body should contain related_meta, got: %s", out)
	}
	if !strings.Contains(out, "ou_xxx") || !strings.Contains(out, "https://feishu.cn/docs/yyy") {
		t.Fatalf("dry-run body should contain related-meta values, got: %s", out)
	}
}

func TestEntityUpdateExecute_RelatedMeta(t *testing.T) {
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
		"--related-meta", `{"classifications":[{"id":"7517595051844222977"}]}`,
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
