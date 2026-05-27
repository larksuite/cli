// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// --- Validate tests ---

func TestDriveInspectValidate_EmptyURL(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	err := DriveInspect.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected error for empty --url, got nil")
	}
}

func TestDriveInspectValidate_UnsupportedURL(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "https://google.com/some/page")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	err := DriveInspect.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected error for unsupported URL, got nil")
	}
}

func TestDriveInspectValidate_NonLarkHostWithLarkPath(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "https://google.com/docx/doxcnLooksValid")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	err := DriveInspect.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("expected no error for non-Lark host with Lark-like path (host validation removed), got %v", err)
	}
}

func TestDriveInspectValidate_BareTokenWithoutType(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "doxcnBareToken")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	err := DriveInspect.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected error for bare token without --type, got nil")
	}
}

func TestDriveInspectValidate_BareTokenWithType(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "doxcnBareToken")
	_ = cmd.Flags().Set("type", "docx")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	err := DriveInspect.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDriveInspectValidate_ValidDocxURL(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "https://xxx.feishu.cn/docx/doxcnABC")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	err := DriveInspect.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDriveInspectValidate_ValidWikiURL(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "https://xxx.feishu.cn/wiki/wikcnABC")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	err := DriveInspect.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDriveInspectValidate_ValidDoubaoDriveFileURL(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "https://feishu.doubao.com/drive/file/boxcnABC")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	err := DriveInspect.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDriveInspectValidate_ValidDoubaoChatDriveFolderURL(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "https://feishu.doubao.com/chat/drive/fldcnABC")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	err := DriveInspect.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDriveInspectValidate_ValidDoubaoDriveShareFolderURL(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "https://feishu.doubao.com/drive/shr/fldcnABC")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	err := DriveInspect.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// --- DryRun tests ---

func TestDriveInspectDryRun_DocxURL(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "https://xxx.feishu.cn/docx/doxcnABC")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	dry := DriveInspect.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			URL    string                 `json:"url"`
			Method string                 `json:"method"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run: %v", err)
	}
	if len(got.API) != 1 {
		t.Fatalf("expected 1 API step, got %d", len(got.API))
	}
	if got.API[0].URL != "/open-apis/drive/v1/metas/batch_query" {
		t.Errorf("API URL = %q, want /open-apis/drive/v1/metas/batch_query", got.API[0].URL)
	}
	// Verify body contains request_docs with the correct token and type.
	reqDocs, ok := got.API[0].Body["request_docs"].([]interface{})
	if !ok || len(reqDocs) != 1 {
		t.Fatalf("expected request_docs with 1 entry, got %v", got.API[0].Body["request_docs"])
	}
	doc, _ := reqDocs[0].(map[string]interface{})
	if doc["doc_token"] != "doxcnABC" {
		t.Errorf("doc_token = %v, want doxcnABC", doc["doc_token"])
	}
	if doc["doc_type"] != "docx" {
		t.Errorf("doc_type = %v, want docx", doc["doc_type"])
	}
}

func TestDriveInspectDryRun_WikiURL(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "https://xxx.feishu.cn/wiki/wikcnABC")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	dry := DriveInspect.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run: %v", err)
	}
	if len(got.API) != 2 {
		t.Fatalf("expected 2 API steps, got %d", len(got.API))
	}
	if got.API[0].URL != "/open-apis/wiki/v2/spaces/get_node" {
		t.Errorf("step 1 URL = %q, want /open-apis/wiki/v2/spaces/get_node", got.API[0].URL)
	}
	// Verify step 1 params contain the wiki token.
	if got.API[0].Params["token"] != "wikcnABC" {
		t.Errorf("step 1 params.token = %v, want wikcnABC", got.API[0].Params["token"])
	}
	if got.API[1].URL != "/open-apis/drive/v1/metas/batch_query" {
		t.Errorf("step 2 URL = %q, want /open-apis/drive/v1/metas/batch_query", got.API[1].URL)
	}
	// Verify step 2 body contains request_docs placeholder.
	if got.API[1].Body["request_docs"] == nil {
		t.Error("step 2 body should contain request_docs")
	}
}

func TestDriveInspectDryRun_BareTokenWithType(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "doxcnBareToken")
	_ = cmd.Flags().Set("type", "docx")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	dry := DriveInspect.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			URL string `json:"url"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run: %v", err)
	}
	if len(got.API) != 1 {
		t.Fatalf("expected 1 API step, got %d", len(got.API))
	}
}

func TestDriveInspectDryRun_DoubaoDriveFileURL(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "https://feishu.doubao.com/drive/file/boxcnABC")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	dry := DriveInspect.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run: %v", err)
	}
	reqDocs, ok := got.API[0].Body["request_docs"].([]interface{})
	if !ok || len(reqDocs) != 1 {
		t.Fatalf("expected request_docs with 1 entry, got %v", got.API[0].Body["request_docs"])
	}
	doc, _ := reqDocs[0].(map[string]interface{})
	if doc["doc_token"] != "boxcnABC" {
		t.Errorf("doc_token = %v, want boxcnABC", doc["doc_token"])
	}
	if doc["doc_type"] != "file" {
		t.Errorf("doc_type = %v, want file", doc["doc_type"])
	}
}

func TestDriveInspectDryRun_DoubaoDriveShareFolderURL(t *testing.T) {
	cmd := &cobra.Command{Use: "drive +inspect"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("url", "https://feishu.doubao.com/drive/shr/fldcnABC")

	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	dry := DriveInspect.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run: %v", err)
	}
	reqDocs, ok := got.API[0].Body["request_docs"].([]interface{})
	if !ok || len(reqDocs) != 1 {
		t.Fatalf("expected request_docs with 1 entry, got %v", got.API[0].Body["request_docs"])
	}
	doc, _ := reqDocs[0].(map[string]interface{})
	if doc["doc_token"] != "fldcnABC" {
		t.Errorf("doc_token = %v, want fldcnABC", doc["doc_token"])
	}
	if doc["doc_type"] != "folder" {
		t.Errorf("doc_type = %v, want folder", doc["doc_type"])
	}
}

// --- Execute tests ---

func TestDriveInspectExecute_DocxURL(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := driveTestConfig()
	f, stdout, _, reg := cmdutil.TestFactory(t, cfg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "doxcnABC", "doc_type": "docx", "title": "Test Doc"},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveInspect, []string{
		"+inspect",
		"--url", "https://xxx.feishu.cn/docx/doxcnABC",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["type"] != "docx" {
		t.Errorf("type = %v, want docx", data["type"])
	}
	if data["token"] != "doxcnABC" {
		t.Errorf("token = %v, want doxcnABC", data["token"])
	}
	if data["title"] != "Test Doc" {
		t.Errorf("title = %v, want Test Doc", data["title"])
	}
	if _, ok := data["wiki_node"]; ok {
		t.Error("wiki_node should not be present for non-wiki URL")
	}
}

func TestDriveInspectExecute_WikiURL(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := driveTestConfig()
	f, stdout, _, reg := cmdutil.TestFactory(t, cfg)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":   "docx",
					"obj_token":  "doxcnUnwrapped",
					"space_id":   "space123",
					"node_token": "wikcnNodeToken",
					"title":      "Wiki Doc",
					"node_type":  "origin",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "doxcnUnwrapped", "doc_type": "docx", "title": "Wiki Doc"},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveInspect, []string{
		"+inspect",
		"--url", "https://xxx.feishu.cn/wiki/wikcnABC",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["type"] != "docx" {
		t.Errorf("type = %v, want docx (unwrapped from wiki)", data["type"])
	}
	if data["token"] != "doxcnUnwrapped" {
		t.Errorf("token = %v, want doxcnUnwrapped", data["token"])
	}
	if data["title"] != "Wiki Doc" {
		t.Errorf("title = %v, want Wiki Doc", data["title"])
	}
	wikiNode, ok := data["wiki_node"].(map[string]interface{})
	if !ok {
		t.Fatal("wiki_node should be present for wiki URL")
	}
	if wikiNode["space_id"] != "space123" {
		t.Errorf("wiki_node.space_id = %v, want space123", wikiNode["space_id"])
	}
}

func TestDriveInspectExecute_WikiGetNodeIncompleteData(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := driveTestConfig()
	f, stdout, _, reg := cmdutil.TestFactory(t, cfg)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "",
					"obj_token": "",
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveInspect, []string{
		"+inspect",
		"--url", "https://xxx.feishu.cn/wiki/wikcnABC",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for incomplete wiki node data, got nil")
	}
}

func TestDriveInspectExecute_BareTokenWithType(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := driveTestConfig()
	f, stdout, _, reg := cmdutil.TestFactory(t, cfg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "doxcnBare", "doc_type": "docx", "title": "Bare Doc"},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveInspect, []string{
		"+inspect",
		"--url", "doxcnBare",
		"--type", "docx",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["type"] != "docx" {
		t.Errorf("type = %v, want docx", data["type"])
	}
	if data["token"] != "doxcnBare" {
		t.Errorf("token = %v, want doxcnBare", data["token"])
	}
}

func TestDriveInspectExecute_BatchQueryError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := driveTestConfig()
	f, stdout, _, reg := cmdutil.TestFactory(t, cfg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 99991668,
			"msg":  "permission denied",
		},
	})

	err := mountAndRunDrive(t, DriveInspect, []string{
		"+inspect",
		"--url", "https://xxx.feishu.cn/docx/doxcnABC",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for batch_query failure, got nil")
	}
}

func TestDriveInspectExecute_PrettyFormat(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := driveTestConfig()
	f, stdout, _, reg := cmdutil.TestFactory(t, cfg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "doxcnABC", "doc_type": "docx", "title": "Test Doc"},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveInspect, []string{
		"+inspect",
		"--url", "https://xxx.feishu.cn/docx/doxcnABC",
		"--format", "pretty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Pretty format outputs to stdout as text, not JSON envelope.
	// Just verify it didn't error.
	_ = stdout
}
