// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestDocsCreateBotAutoGrantSuccess(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, "ou_current_user"))
	registerDocsCreateMCPStub(reg, map[string]interface{}{
		"doc_id":  "doxcn_new_doc",
		"doc_url": "https://example.feishu.cn/docx/doxcn_new_doc",
		"message": "文档创建成功",
	})

	permStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/doxcn_new_doc/members",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"member": map[string]interface{}{
					"member_id":   "ou_current_user",
					"member_type": "openid",
					"perm":        "full_access",
				},
			},
		},
	}
	reg.Register(permStub)

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "项目计划",
		"--markdown", "## 目标",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDocsCreateEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantGranted {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantGranted)
	}
	if grant["user_open_id"] != "ou_current_user" {
		t.Fatalf("permission_grant.user_open_id = %#v, want %q", grant["user_open_id"], "ou_current_user")
	}
	if grant["message"] != "Granted the current CLI user full_access (可管理权限) on the new document." {
		t.Fatalf("permission_grant.message = %#v", grant["message"])
	}

	var body map[string]interface{}
	if err := json.Unmarshal(permStub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to parse permission request body: %v", err)
	}
	if body["member_type"] != "openid" || body["member_id"] != "ou_current_user" || body["perm"] != "full_access" || body["type"] != "user" {
		t.Fatalf("unexpected permission request body: %#v", body)
	}
}

func TestDocsCreateBotAutoGrantSkippedWithoutCurrentUser(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	registerDocsCreateMCPStub(reg, map[string]interface{}{
		"doc_id":  "doxcn_new_doc",
		"doc_url": "https://example.feishu.cn/docx/doxcn_new_doc",
		"message": "文档创建成功",
	})

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--markdown", "## 内容",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDocsCreateEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantSkipped {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantSkipped)
	}
	if _, ok := grant["user_open_id"]; ok {
		t.Fatalf("did not expect user_open_id when current user is missing: %#v", grant)
	}
}

func TestDocsCreateUserSkipsPermissionGrantAugmentation(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, "ou_current_user"))
	registerDocsCreateMCPStub(reg, map[string]interface{}{
		"doc_id":  "doxcn_new_doc",
		"doc_url": "https://example.feishu.cn/docx/doxcn_new_doc",
		"message": "文档创建成功",
	})

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--markdown", "## 内容",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDocsCreateEnvelope(t, stdout)
	if _, ok := data["permission_grant"]; ok {
		t.Fatalf("did not expect permission_grant in user mode output: %#v", data)
	}
}

func TestDocsCreateBotAutoGrantFailureDoesNotFailCreate(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, "ou_current_user"))
	registerDocsCreateMCPStub(reg, map[string]interface{}{
		"doc_id":  "doxcn_new_doc",
		"doc_url": "https://example.feishu.cn/wiki/wikcn_new_node",
		"message": "文档创建成功",
	})

	permStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/wikcn_new_node/members",
		Body: map[string]interface{}{
			"code": 230001,
			"msg":  "no permission",
		},
	}
	reg.Register(permStub)

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--markdown", "## 内容",
		"--wiki-space", "my_library",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("document creation should still succeed when auto-grant fails, got: %v", err)
	}

	data := decodeDocsCreateEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantFailed {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantFailed)
	}
	if !strings.Contains(grant["message"].(string), "full_access (可管理权限)") {
		t.Fatalf("permission_grant.message = %q, want permission hint", grant["message"])
	}
	if !strings.Contains(grant["message"].(string), "retry later") {
		t.Fatalf("permission_grant.message = %q, want retry guidance", grant["message"])
	}

	var body map[string]interface{}
	if err := json.Unmarshal(permStub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to parse permission request body: %v", err)
	}
	if body["perm_type"] != "container" {
		t.Fatalf("permission request perm_type = %#v, want %q", body["perm_type"], "container")
	}
}

func docsCreateTestConfig(t *testing.T, userOpenID string) *core.CliConfig {
	t.Helper()

	replacer := strings.NewReplacer("/", "-", " ", "-")
	suffix := replacer.Replace(strings.ToLower(t.Name()))
	return &core.CliConfig{
		AppID:      "test-docs-create-" + suffix,
		AppSecret:  "secret-docs-create-" + suffix,
		Brand:      core.BrandFeishu,
		UserOpenId: userOpenID,
	}
}

func registerDocsCreateMCPStub(reg *httpmock.Registry, result map[string]interface{}) {
	payload, _ := json.Marshal(result)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/mcp",
		Body: map[string]interface{}{
			"result": map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": string(payload),
					},
				},
			},
		},
	})
}

func runDocsCreateShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()

	parent := &cobra.Command{Use: "docs"}
	DocsCreate.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func decodeDocsCreateEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode output: %v\nraw=%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("missing data in output envelope: %#v", envelope)
	}
	return data
}
