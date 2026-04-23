// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"bytes"
	"encoding/json"
	"mime"
	"mime/multipart"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// TestSlidesMediaUploadBasic verifies the happy path: token + presentation_id
// with a real (small) local file.
func TestSlidesMediaUploadBasic(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	if err := os.WriteFile("img.png", []byte("png-bytes"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_tok_xyz"},
		},
	}
	reg.Register(uploadStub)

	err := runSlidesShortcut(t, f, stdout, SlidesMediaUpload, []string{
		"+media-upload",
		"--file", "img.png",
		"--presentation", "pres_abc",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	if data["file_token"] != "file_tok_xyz" {
		t.Fatalf("file_token = %v, want file_tok_xyz", data["file_token"])
	}
	if data["presentation_id"] != "pres_abc" {
		t.Fatalf("presentation_id = %v, want pres_abc", data["presentation_id"])
	}
	if data["file_name"] != "img.png" {
		t.Fatalf("file_name = %v, want img.png", data["file_name"])
	}
	if data["size"] != float64(len("png-bytes")) {
		t.Fatalf("size = %v, want %d", data["size"], len("png-bytes"))
	}

	body := decodeMultipartBody(t, uploadStub)
	if got := body.Fields["parent_type"]; got != slidesMediaParentType {
		t.Fatalf("parent_type = %q, want %q", got, slidesMediaParentType)
	}
	if got := body.Fields["parent_node"]; got != "pres_abc" {
		t.Fatalf("parent_node = %q, want pres_abc", got)
	}
	if got := body.Fields["file_name"]; got != "img.png" {
		t.Fatalf("file_name = %q, want img.png", got)
	}
}

// TestSlidesMediaUploadFromSlidesURL verifies that a slides URL is accepted
// and the path-segment token is used as parent_node.
func TestSlidesMediaUploadFromSlidesURL(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)
	if err := os.WriteFile("p.png", []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"file_token": "tok"}},
	}
	reg.Register(stub)

	err := runSlidesShortcut(t, f, stdout, SlidesMediaUpload, []string{
		"+media-upload",
		"--file", "p.png",
		"--presentation", "https://x.feishu.cn/slides/url_token_123?from=share",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeMultipartBody(t, stub)
	if got := body.Fields["parent_node"]; got != "url_token_123" {
		t.Fatalf("parent_node = %q, want url_token_123", got)
	}

	data := decodeShortcutData(t, stdout)
	if data["presentation_id"] != "url_token_123" {
		t.Fatalf("presentation_id = %v, want url_token_123", data["presentation_id"])
	}
}

// TestSlidesMediaUploadFromWikiURL verifies wiki URL → get_node lookup is performed
// and the resolved obj_token is used as parent_node.
func TestSlidesMediaUploadFromWikiURL(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)
	if err := os.WriteFile("w.png", []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "slides",
					"obj_token": "real_pres_id",
				},
			},
		},
	})
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"file_token": "tok"}},
	}
	reg.Register(uploadStub)

	err := runSlidesShortcut(t, f, stdout, SlidesMediaUpload, []string{
		"+media-upload",
		"--file", "w.png",
		"--presentation", "https://x.feishu.cn/wiki/wikcn_xyz",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeMultipartBody(t, uploadStub)
	if got := body.Fields["parent_node"]; got != "real_pres_id" {
		t.Fatalf("parent_node = %q, want real_pres_id", got)
	}
}

// TestSlidesMediaUploadWikiWrongType verifies wiki resolution rejects non-slides docs.
func TestSlidesMediaUploadWikiWrongType(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)
	if err := os.WriteFile("w.png", []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "docx",
					"obj_token": "docx_tok",
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaUpload, []string{
		"+media-upload",
		"--file", "w.png",
		"--presentation", "https://x.feishu.cn/wiki/wikcn",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error for non-slides wiki node")
	}
	if !strings.Contains(err.Error(), "docx") {
		t.Fatalf("err = %v, want mention of resolved obj_type", err)
	}
}

// TestSlidesMediaUploadFileNotFound verifies a missing local file fails fast.
func TestSlidesMediaUploadFileNotFound(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesMediaUpload, []string{
		"+media-upload",
		"--file", "missing.png",
		"--presentation", "pres_abc",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "file not found") && !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("err = %v, want file-not-found error", err)
	}
}

// TestSlidesMediaUploadInvalidPresentation verifies validation rejects a bad ref.
func TestSlidesMediaUploadInvalidPresentation(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesMediaUpload, []string{
		"+media-upload",
		"--file", "any.png",
		"--presentation", "https://x.feishu.cn/docx/foo",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected validation error for unsupported presentation URL")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("err = %v, want 'unsupported' mention", err)
	}
}

// TestSlidesMediaUploadDryRun verifies dry-run prints the upload step.
func TestSlidesMediaUploadDryRun(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)
	if err := os.WriteFile("dry.png", []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesMediaUpload, []string{
		"+media-upload",
		"--file", "dry.png",
		"--presentation", "pres_abc",
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/drive/v1/medias/upload_all") {
		t.Fatalf("dry-run should mention upload_all, got: %s", out)
	}
	if !strings.Contains(out, slidesMediaParentType) {
		t.Fatalf("dry-run should mention parent_type %q, got: %s", slidesMediaParentType, out)
	}
}

// runSlidesShortcut mounts and executes a slides shortcut with the given args.
func runSlidesShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, sc common.Shortcut, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "slides"}
	sc.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// decodeShortcutData parses the JSON envelope and returns the data map.
func decodeShortcutData(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\nraw=%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("missing data: %#v", envelope)
	}
	return data
}

// withSlidesTestWorkingDir chdirs to dir for this test (restored on cleanup).
// Not compatible with t.Parallel — chdir is process-wide.
func withSlidesTestWorkingDir(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
}

type capturedMultipart struct {
	Fields map[string]string
	Files  map[string][]byte
}

func decodeMultipartBody(t *testing.T, stub *httpmock.Stub) capturedMultipart {
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
	body := capturedMultipart{Fields: map[string]string{}, Files: map[string][]byte{}}
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		data := readAll(t, part)
		if part.FileName() != "" {
			body.Files[part.FormName()] = data
			continue
		}
		body.Fields[part.FormName()] = string(data)
	}
	return body
}

func readAll(t *testing.T, r interface {
	Read(p []byte) (n int, err error)
}) []byte {
	t.Helper()
	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			break
		}
	}
	return buf.Bytes()
}
