// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime"
	"mime/multipart"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func runParseImageShortcut(t *testing.T, args []string, factory *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "whiteboard"}
	WhiteboardParseImage.Mount(parent, factory)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func parseImageTestFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{
		AppID:      "test-parse-image",
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_testuser",
	}
	factory, stdout, _, reg := cmdutil.TestFactory(t, cfg)
	return factory, stdout, reg
}

func TestWhiteboardParseImageDryRun_RequestShape(t *testing.T) {
	factory, stdout, _ := parseImageTestFactory(t)

	err := runParseImageShortcut(t, []string{
		"+parse-image",
		"--whiteboard-token", "test-board",
		"--image", "./diagram.png",
		"--mode", "agentic_max",
		"--idempotent-token", "parse-token-12345",
		"--overwrite",
		"--dry-run",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "POST") {
		t.Fatalf("dry-run output should contain POST, got: %s", out)
	}
	if !strings.Contains(out, "/open-apis/board/v1/whiteboards/test...oard/parse_image") {
		t.Fatalf("dry-run output should contain masked parse_image URL, got: %s", out)
	}
	if !strings.Contains(out, "image_file") || !strings.Contains(out, "@./diagram.png") {
		t.Fatalf("dry-run output should describe image_file form upload, got: %s", out)
	}
	if !strings.Contains(out, "parse-token-12345") || !strings.Contains(out, "overwrite") || !strings.Contains(out, "agentic_max") {
		t.Fatalf("dry-run output should include client_token, overwrite, and mode, got: %s", out)
	}
}

func TestWhiteboardParseImageDryRun_OmitsModeByDefault(t *testing.T) {
	factory, stdout, _ := parseImageTestFactory(t)

	err := runParseImageShortcut(t, []string{
		"+parse-image",
		"--whiteboard-token", "test-board",
		"--image", "./diagram.png",
		"--idempotent-token", "parse-token-12345",
		"--dry-run",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stdout.String(), `"mode"`) {
		t.Fatalf("dry-run mode should be omitted when --mode is not provided: %s", stdout.String())
	}
}

func TestWhiteboardParseImageExecute_PostsMultipartAndOutputsNextCommand(t *testing.T) {
	factory, stdout, reg := parseImageTestFactory(t)
	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)
	if err := os.WriteFile("diagram.png", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 'x'}, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-board/parse_image",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"task_id": "7670001",
				"status":  "pending",
			},
		},
	}
	reg.Register(stub)

	err := runParseImageShortcut(t, []string{
		"+parse-image",
		"--whiteboard-token", "test-board",
		"--image", "./diagram.png",
		"--mode", "agentic",
		"--idempotent-token", "parse-token-12345",
		"--overwrite",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeParseImageMultipart(t, stub)
	if got := body.Fields["client_token"]; got != "parse-token-12345" {
		t.Fatalf("client_token = %q, want parse-token-12345", got)
	}
	if got := body.Fields["overwrite"]; got != "true" {
		t.Fatalf("overwrite = %q, want true", got)
	}
	if got := body.Fields["mode"]; got != "agentic" {
		t.Fatalf("mode = %q, want agentic", got)
	}
	if got := string(body.Files["image_file"]); !strings.Contains(got, "PNG") {
		t.Fatalf("image_file body = %q, want PNG bytes", got)
	}
	if got := body.Names["image_file"]; got != "diagram.png" {
		t.Fatalf("image_file filename = %q, want diagram.png", got)
	}

	data := decodeParseImageEnvelope(t, stdout)
	if data["task_id"] != "7670001" || data["status"] != "pending" {
		t.Fatalf("unexpected output data: %#v", data)
	}
	next, _ := data["next_command"].(string)
	if !strings.Contains(next, "whiteboard +parse-image-result") ||
		!strings.Contains(next, "--whiteboard-token test-board") ||
		!strings.Contains(next, "--task-id 7670001") ||
		!strings.Contains(next, "--wait") ||
		!strings.Contains(next, "--format json") {
		t.Fatalf("next_command = %q, want parse-image-result resume command", next)
	}
}

func TestWhiteboardParseImageExecute_AcceptsImageShorthand(t *testing.T) {
	factory, stdout, reg := parseImageTestFactory(t)
	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)
	if err := os.WriteFile("diagram.png", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-board/parse_image",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"task_id": "7670002",
				"status":  "pending",
			},
		},
	}
	reg.Register(stub)

	err := runParseImageShortcut(t, []string{
		"+parse-image",
		"--whiteboard-token", "test-board",
		"-i", "./diagram.png",
		"--idempotent-token", "parse-token-12345",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeParseImageEnvelope(t, stdout)
	if data["task_id"] != "7670002" {
		t.Fatalf("task_id = %#v, want 7670002", data["task_id"])
	}
	body := decodeParseImageMultipart(t, stub)
	if _, ok := body.Fields["mode"]; ok {
		t.Fatalf("mode should be omitted when --mode is not provided: %#v", body.Fields)
	}
}

func TestWhiteboardParseImageExecute_PreservesBotIdentityInNextCommand(t *testing.T) {
	factory, stdout, reg := parseImageTestFactory(t)
	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)
	if err := os.WriteFile("diagram.png", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-board/parse_image",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"task_id": "7670003",
				"status":  "pending",
			},
		},
	})

	err := runParseImageShortcut(t, []string{
		"+parse-image",
		"--whiteboard-token", "test-board",
		"--image", "./diagram.png",
		"--idempotent-token", "parse-token-12345",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeParseImageEnvelope(t, stdout)
	next, _ := data["next_command"].(string)
	if !strings.Contains(next, "--as bot") ||
		!strings.Contains(next, "--wait") ||
		!strings.Contains(next, "--format json") {
		t.Fatalf("next_command = %q, want bot identity", next)
	}
}

func TestWhiteboardParseImageValidateRejectsUnsupportedImage(t *testing.T) {
	factory, stdout, _ := parseImageTestFactory(t)
	err := runParseImageShortcut(t, []string{
		"+parse-image",
		"--whiteboard-token", "test-board",
		"--image", "./diagram.pdf",
		"--as", "user",
	}, factory, stdout)
	assertValidationParam(t, err, "--image", false)
}

func TestWhiteboardParseImageValidateRejectsInvalidMode(t *testing.T) {
	factory, stdout, _ := parseImageTestFactory(t)
	err := runParseImageShortcut(t, []string{
		"+parse-image",
		"--whiteboard-token", "test-board",
		"--image", "./diagram.png",
		"--mode", "slow",
		"--as", "user",
	}, factory, stdout)
	assertValidationParam(t, err, "--mode", false)
}

func TestWhiteboardParseImageValidateRejectsExplicitEmptyMode(t *testing.T) {
	factory, stdout, _ := parseImageTestFactory(t)
	err := runParseImageShortcut(t, []string{
		"+parse-image",
		"--whiteboard-token", "test-board",
		"--image", "./diagram.png",
		"--mode=",
		"--as", "user",
	}, factory, stdout)
	assertValidationParam(t, err, "--mode", false)
}

func TestWhiteboardParseImageExecuteRejectsMissingTaskID(t *testing.T) {
	factory, stdout, reg := parseImageTestFactory(t)
	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)
	if err := os.WriteFile("diagram.png", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-board/parse_image",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	})

	err := runParseImageShortcut(t, []string{
		"+parse-image",
		"--whiteboard-token", "test-board",
		"--image", "./diagram.png",
		"--idempotent-token", "parse-token-12345",
		"--as", "user",
	}, factory, stdout)
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("error type = %T, want *errs.InternalError", err)
	}
	if internalErr.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("Subtype = %q, want %q", internalErr.Subtype, errs.SubtypeInvalidResponse)
	}
}

type capturedParseImageMultipart struct {
	Fields map[string]string
	Files  map[string][]byte
	Names  map[string]string
}

func decodeParseImageMultipart(t *testing.T, stub *httpmock.Stub) capturedParseImageMultipart {
	t.Helper()
	contentType := stub.CapturedHeaders.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parse content-type %q: %v", contentType, err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q, want multipart/form-data", mediaType)
	}
	reader := multipart.NewReader(bytes.NewReader(stub.CapturedBody), params["boundary"])
	body := capturedParseImageMultipart{Fields: map[string]string{}, Files: map[string][]byte{}, Names: map[string]string{}}
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(part); err != nil {
			t.Fatalf("read multipart part: %v", err)
		}
		if part.FileName() != "" {
			body.Files[part.FormName()] = buf.Bytes()
			body.Names[part.FormName()] = part.FileName()
		} else {
			body.Fields[part.FormName()] = buf.String()
		}
	}
	return body
}

func decodeParseImageEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var envelope struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout envelope: %v\nstdout=%s", err, stdout.String())
	}
	if !envelope.OK {
		t.Fatalf("ok = false, stdout=%s", stdout.String())
	}
	return envelope.Data
}
