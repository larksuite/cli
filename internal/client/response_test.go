// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/vfs/localfileio"
)

func newApiResp(body []byte, headers map[string]string) *larkcore.ApiResp {
	return newApiRespWithStatus(200, body, headers)
}

func newApiRespWithStatus(status int, body []byte, headers map[string]string) *larkcore.ApiResp {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &larkcore.ApiResp{
		StatusCode: status,
		Header:     h,
		RawBody:    body,
	}
}

func TestIsJSONContentType_Extended(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"text/json", true},
		{"application/octet-stream", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsJSONContentType(tt.ct); got != tt.want {
			t.Errorf("IsJSONContentType(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}

func TestParseJSONResponse(t *testing.T) {
	body := []byte(`{"code":0,"msg":"ok","data":{"id":"123"}}`)
	resp := newApiResp(body, map[string]string{"Content-Type": "application/json"})
	result, err := ParseJSONResponse(resp)
	if err != nil {
		t.Fatalf("ParseJSONResponse failed: %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("expected map result")
	}
	if m["msg"] != "ok" {
		t.Errorf("expected msg=ok, got %v", m["msg"])
	}
}

func TestParseJSONResponse_Invalid(t *testing.T) {
	resp := newApiResp([]byte(`not json`), map[string]string{"Content-Type": "application/json"})
	_, err := ParseJSONResponse(resp)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseJSONResponse_EmptyBody_WrapsEOF(t *testing.T) {
	resp := newApiResp([]byte{}, map[string]string{"Content-Type": "application/json"})
	_, err := ParseJSONResponse(resp)
	if err == nil {
		t.Fatal("expected error for empty body")
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected wrapped io.EOF, got %v", err)
	}
}

func TestResolveFilename(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{
			"from content-type pdf",
			map[string]string{"Content-Type": "application/pdf"},
			"download.pdf",
		},
		{
			"from content-type png",
			map[string]string{"Content-Type": "image/png"},
			"download.png",
		},
		{
			"unknown type",
			map[string]string{"Content-Type": "application/octet-stream"},
			"download.bin",
		},
		{
			"empty content-type",
			map[string]string{},
			"download.bin",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := newApiResp([]byte("data"), tt.headers)
			got := ResolveFilename(resp)
			if got != tt.want {
				t.Errorf("ResolveFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMimeToExt_Extended(t *testing.T) {
	tests := []struct {
		ct   string
		want string
	}{
		{"application/pdf", ".pdf"},
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"image/gif", ".gif"},
		{"text/plain", ".txt"},
		{"text/csv", ".csv"},
		{"text/html", ".html"},
		{"application/zip", ".zip"},
		{"application/xml", ".xml"},
		{"text/xml", ".xml"},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx"},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"},
		{"application/vnd.openxmlformats-officedocument.presentationml.presentation", ".pptx"},
		{"application/octet-stream", ".bin"},
		{"", ".bin"},
	}
	for _, tt := range tests {
		if got := mimeToExt(tt.ct); got != tt.want {
			t.Errorf("mimeToExt(%q) = %q, want %q", tt.ct, got, tt.want)
		}
	}
}

func TestSaveResponse(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	body := []byte("hello binary data")
	resp := newApiResp(body, map[string]string{"Content-Type": "application/octet-stream"})

	meta, err := SaveResponse(&localfileio.LocalFileIO{}, resp, "test_output.bin")
	if err != nil {
		t.Fatalf("SaveResponse failed: %v", err)
	}
	if meta["size_bytes"] != int64(len(body)) {
		t.Errorf("expected size_bytes=%d, got %v", len(body), meta["size_bytes"])
	}

	savedPath, _ := meta["saved_path"].(string)
	data, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if !bytes.Equal(data, body) {
		t.Errorf("saved content mismatch")
	}
}

func TestSaveResponse_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	resp := newApiResp([]byte("data"), map[string]string{"Content-Type": "application/octet-stream"})

	meta, err := SaveResponse(&localfileio.LocalFileIO{}, resp, filepath.Join("sub", "deep", "out.bin"))
	if err != nil {
		t.Fatalf("SaveResponse with nested dir failed: %v", err)
	}
	savedPath, _ := meta["saved_path"].(string)
	if _, err := os.Stat(savedPath); err != nil {
		t.Errorf("expected file to exist at %s", savedPath)
	}
}

func TestHandleResponse_JSON(t *testing.T) {
	body := []byte(`{"code":0,"msg":"ok","data":{"id":"1"}}`)
	resp := newApiResp(body, map[string]string{"Content-Type": "application/json"})

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := HandleResponse(resp, ResponseOptions{
		Out:    &out,
		ErrOut: &errOut,
		FileIO: &localfileio.LocalFileIO{},
	})
	if err != nil {
		t.Fatalf("HandleResponse failed: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"code"`)) {
		t.Errorf("expected JSON output, got: %s", out.String())
	}
}

func TestHandleResponse_JSONWithError(t *testing.T) {
	body := []byte(`{"code":99991400,"msg":"invalid token"}`)
	resp := newApiResp(body, map[string]string{"Content-Type": "application/json"})

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := HandleResponse(resp, ResponseOptions{
		Out:    &out,
		ErrOut: &errOut,
		FileIO: &localfileio.LocalFileIO{},
	})
	if err == nil {
		t.Error("expected error for non-zero code")
	}
}

func TestHandleResponse_EmptyJSONBody_ShowsDiagnostic(t *testing.T) {
	resp := newApiResp([]byte{}, map[string]string{"Content-Type": "application/json"})

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := HandleResponse(resp, ResponseOptions{
		Out:    &out,
		ErrOut: &errOut,
	})
	if err == nil {
		t.Fatal("expected error for empty JSON body")
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitAPI {
		t.Fatalf("expected ExitAPI, got %d", exitErr.Code)
	}
	if exitErr.Detail == nil {
		t.Fatal("expected detail on exit error")
	}
	if exitErr.Detail.Message != "API returned an empty JSON response body" {
		t.Fatalf("unexpected message: %q", exitErr.Detail.Message)
	}
	if !strings.Contains(exitErr.Detail.Hint, "--output") {
		t.Fatalf("expected hint to mention --output, got %q", exitErr.Detail.Hint)
	}
}

func TestHandleResponse_BinaryAutoSave(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	resp := newApiResp([]byte("PNG DATA"), map[string]string{"Content-Type": "image/png"})

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := HandleResponse(resp, ResponseOptions{
		Out:    &out,
		ErrOut: &errOut,
		FileIO: &localfileio.LocalFileIO{},
	})
	if err != nil {
		t.Fatalf("HandleResponse binary failed: %v", err)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("binary response detected")) {
		t.Errorf("expected binary detection message, got: %s", errOut.String())
	}
}

func TestHandleResponse_BinaryWithOutput(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	resp := newApiResp([]byte("PNG DATA"), map[string]string{"Content-Type": "image/png"})

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := HandleResponse(resp, ResponseOptions{
		OutputPath: "out.png",
		Out:        &out,
		ErrOut:     &errOut,
		FileIO:     &localfileio.LocalFileIO{},
	})
	if err != nil {
		t.Fatalf("HandleResponse with output path failed: %v", err)
	}
	data, _ := os.ReadFile("out.png")
	if string(data) != "PNG DATA" {
		t.Errorf("expected saved PNG DATA, got: %s", data)
	}
}

func TestHandleResponse_NonJSONError_404(t *testing.T) {
	resp := newApiRespWithStatus(404, []byte("404 page not found"), map[string]string{"Content-Type": "text/plain"})

	var out, errOut bytes.Buffer
	err := HandleResponse(resp, ResponseOptions{Out: &out, ErrOut: &errOut, FileIO: &localfileio.LocalFileIO{}})
	if err == nil {
		t.Fatal("expected error for 404 text/plain")
	}
	got := err.Error()
	if !strings.Contains(got, "HTTP 404") || !strings.Contains(got, "404 page not found") {
		t.Errorf("expected 'HTTP 404: 404 page not found', got: %s", got)
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitAPI {
		t.Errorf("expected ExitAPI (%d) for 4xx, got code: %d", output.ExitAPI, exitErr.Code)
	}
}

func TestHandleResponse_NonJSONError_502(t *testing.T) {
	resp := newApiRespWithStatus(502, []byte("<html>Bad Gateway</html>"), map[string]string{"Content-Type": "text/html"})

	var out, errOut bytes.Buffer
	err := HandleResponse(resp, ResponseOptions{Out: &out, ErrOut: &errOut, FileIO: &localfileio.LocalFileIO{}})
	if err == nil {
		t.Fatal("expected error for 502 text/html")
	}
	got := err.Error()
	if !strings.Contains(got, "HTTP 502") || !strings.Contains(got, "Bad Gateway") {
		t.Errorf("expected 'HTTP 502' and 'Bad Gateway' in error, got: %s", got)
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitNetwork {
		t.Errorf("expected ExitNetwork (%d) for 5xx, got code: %d", output.ExitNetwork, exitErr.Code)
	}
}

func TestHandleResponse_200TextPlain_SavesFile(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	resp := newApiRespWithStatus(200, []byte("plain text file content"), map[string]string{"Content-Type": "text/plain"})

	var out, errOut bytes.Buffer
	err := HandleResponse(resp, ResponseOptions{Out: &out, ErrOut: &errOut, FileIO: &localfileio.LocalFileIO{}})
	if err != nil {
		t.Fatalf("expected no error for 200 text/plain, got: %v", err)
	}
	if !strings.Contains(errOut.String(), "binary response detected") {
		t.Errorf("expected binary detection message, got: %s", errOut.String())
	}
}

func TestHandleResponse_BinaryWithJq_RejectsNonJSON(t *testing.T) {
	resp := newApiResp([]byte("PNG DATA"), map[string]string{"Content-Type": "image/png"})

	var out, errOut bytes.Buffer
	err := HandleResponse(resp, ResponseOptions{
		JqExpr: ".data",
		Out:    &out,
		ErrOut: &errOut,
	})
	if err == nil {
		t.Fatal("expected error when --jq is used with non-JSON response")
	}
	if !strings.Contains(err.Error(), "--jq requires a JSON response") {
		t.Errorf("expected '--jq requires a JSON response' error, got: %v", err)
	}
}

func TestSaveResponse_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	resp := newApiResp([]byte("data"), map[string]string{"Content-Type": "application/octet-stream"})
	_, err := SaveResponse(&localfileio.LocalFileIO{}, resp, "../../evil.txt")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "unsafe output path") {
		t.Errorf("expected 'unsafe output path' wrapper, got: %v", err)
	}
}

func TestSaveResponse_RejectsAbsolutePath(t *testing.T) {
	resp := newApiResp([]byte("data"), map[string]string{"Content-Type": "application/octet-stream"})
	_, err := SaveResponse(&localfileio.LocalFileIO{}, resp, "/tmp/evil.txt")
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
}

func TestSaveResponse_MetadataContainsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	resp := newApiResp([]byte("x"), map[string]string{"Content-Type": "text/plain"})
	meta, err := SaveResponse(&localfileio.LocalFileIO{}, resp, "rel.txt")
	if err != nil {
		t.Fatalf("SaveResponse failed: %v", err)
	}
	savedPath, _ := meta["saved_path"].(string)
	if !filepath.IsAbs(savedPath) {
		t.Errorf("saved_path should be absolute, got %q", savedPath)
	}
}

func TestHandleResponse_403JSON_CheckLarkResponse(t *testing.T) {
	body := []byte(`{"code":99991400,"msg":"invalid token"}`)
	resp := newApiRespWithStatus(403, body, map[string]string{"Content-Type": "application/json"})

	var out, errOut bytes.Buffer
	err := HandleResponse(resp, ResponseOptions{Out: &out, ErrOut: &errOut, FileIO: &localfileio.LocalFileIO{}})
	if err == nil {
		t.Fatal("expected error for 403 JSON with non-zero code")
	}
	if !strings.Contains(err.Error(), "99991400") {
		t.Errorf("expected lark error code in message, got: %s", err.Error())
	}
}
