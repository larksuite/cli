// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

func TestNewCmdAuthQRCode_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *QRCodeOptions
	cmd := NewCmdAuthQRCode(f, func(opts *QRCodeOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"https://example.com", "--output", "qr.png", "--size", "128"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.URL != "https://example.com" {
		t.Errorf("URL = %q, want %q", gotOpts.URL, "https://example.com")
	}
	if gotOpts.Size != 128 {
		t.Errorf("Size = %d, want %d", gotOpts.Size, 128)
	}
	if gotOpts.Output != "qr.png" {
		t.Errorf("Output = %q, want %q", gotOpts.Output, "qr.png")
	}
	if gotOpts.ASCII {
		t.Error("ASCII should be false by default")
	}
}

func TestNewCmdAuthQRCode_ASCIIFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *QRCodeOptions
	cmd := NewCmdAuthQRCode(f, func(opts *QRCodeOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"https://example.com", "--ascii"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotOpts.ASCII {
		t.Error("ASCII should be true when --ascii is passed")
	}
}

func TestNewCmdAuthQRCode_DefaultSize(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *QRCodeOptions
	cmd := NewCmdAuthQRCode(f, func(opts *QRCodeOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"https://example.com", "--ascii"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Size != 256 {
		t.Errorf("default Size = %d, want 256", gotOpts.Size)
	}
}

func TestNewCmdAuthQRCode_ExactOneArg(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	cmd := NewCmdAuthQRCode(f, nil)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when no URL argument provided")
	}
}

func TestNewCmdAuthQRCode_RunE_PNGEndToEnd(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(oldWd) })

	cmd := NewCmdAuthQRCode(f, nil)
	cmd.SetArgs([]string{"https://example.com", "--output", "qr.png"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile("qr.png")
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}
	if string(data[:4]) != "\x89PNG" {
		t.Errorf("output does not start with PNG magic bytes, got %x", data[:4])
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v, got: %s", err, stdout.String())
	}
	if result["ok"] != true {
		t.Errorf("ok = %v, want true", result["ok"])
	}
	hint, _ := result["hint"].(string)
	if hint == "" {
		t.Error("hint is empty")
	}
	if !strings.Contains(hint, "MUST include") {
		t.Errorf("hint missing 'MUST include', got: %s", hint)
	}
	if !strings.Contains(hint, "NOT enough") {
		t.Errorf("hint missing 'NOT enough', got: %s", hint)
	}
}

func TestNewCmdAuthQRCode_RunE_MissingOutput(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	cmd := NewCmdAuthQRCode(f, nil)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"https://example.com"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when --output is missing in PNG mode")
	}
}

func TestNewCmdAuthQRCode_HelpText(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	cmd := NewCmdAuthQRCode(f, nil)
	cmd.SetOut(stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := stdout.String()
	for _, want := range []string{
		"qrcode <url>",
		"QR code",
		"--output",
		"--ascii",
		"relative path",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("help missing %q", want)
		}
	}
}

func TestRunQRCode_MissingURL(t *testing.T) {
	err := runQRCode(&QRCodeOptions{URL: ""})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", exitErr.Code, output.ExitValidation)
	}
	if exitErr.Detail.Type != "missing_url" {
		t.Errorf("error type = %q, want %q", exitErr.Detail.Type, "missing_url")
	}
}

func TestRunQRCode_MissingOutput(t *testing.T) {
	err := runQRCode(&QRCodeOptions{URL: "https://example.com", Size: 256})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", exitErr.Code, output.ExitValidation)
	}
	if exitErr.Detail.Type != "missing_output" {
		t.Errorf("error type = %q, want %q", exitErr.Detail.Type, "missing_output")
	}
}

func TestRunQRCode_InvalidSize(t *testing.T) {
	err := runQRCode(&QRCodeOptions{
		URL:    "https://example.com",
		Size:   16,
		Output: "qr.png",
	})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", exitErr.Code, output.ExitValidation)
	}
	if exitErr.Detail.Type != "invalid_size" {
		t.Errorf("error type = %q, want %q", exitErr.Detail.Type, "invalid_size")
	}
}

func TestRunQRCode_SizeTooLarge(t *testing.T) {
	err := runQRCode(&QRCodeOptions{
		URL:    "https://example.com",
		Size:   2048,
		Output: "qr.png",
	})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", exitErr.Code, output.ExitValidation)
	}
	if exitErr.Detail.Type != "invalid_size" {
		t.Errorf("error type = %q, want %q", exitErr.Detail.Type, "invalid_size")
	}
}

func TestRunQRCode_UnsafeOutputPath(t *testing.T) {
	err := runQRCode(&QRCodeOptions{
		URL:    "https://example.com",
		Size:   256,
		Output: "/etc/passwd",
	})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", exitErr.Code, output.ExitValidation)
	}
}

func TestRunQRCode_PNGWritesFile(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(oldWd) })

	err := runQRCode(&QRCodeOptions{
		URL:     "https://example.com",
		Size:    256,
		Output:  "qr.png",
		Factory: f,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat("qr.png")
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output file is empty")
	}

	var result map[string]interface{}
	if jsonErr := json.Unmarshal(stdout.Bytes(), &result); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %v, got: %s", jsonErr, stdout.String())
	}
	if result["ok"] != true {
		t.Errorf("ok = %v, want true", result["ok"])
	}
}

func TestRunQRCode_ASCIIOutputsToStdout(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	err := runQRCode(&QRCodeOptions{
		URL:     "https://example.com",
		ASCII:   true,
		Factory: f,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stdout.Len() == 0 {
		t.Error("ASCII QR code produced no output")
	}
}

func TestGenerateImageQRCode_Success(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test-qr.png")

	if err := generateImageQRCode("https://example.com", 256, outputPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if len(data) == 0 {
		t.Error("output file is empty")
	}
	if len(data) < 8 {
		t.Error("output too small to be a valid PNG")
	}
	if string(data[:4]) != "\x89PNG" {
		t.Errorf("output does not start with PNG magic bytes, got %x", data[:4])
	}
}

func TestGenerateImageQRCode_WriteError(t *testing.T) {
	err := generateImageQRCode("https://example.com", 256, "/nonexistent/deep/nested/dir/qr.png")
	if err == nil {
		t.Fatal("expected error writing to nonexistent directory")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitInternal {
		t.Errorf("exit code = %d, want %d", exitErr.Code, output.ExitInternal)
	}
	if exitErr.Detail.Type != "write_error" {
		t.Errorf("error type = %q, want %q", exitErr.Detail.Type, "write_error")
	}
}

func TestGenerateASCIIQRCode_Success(t *testing.T) {
	var buf strings.Builder
	err := generateASCIIQRCode("https://example.com", &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("ASCII QR code produced no output")
	}
}

func TestGenerateASCIIQRCode_EmptyString(t *testing.T) {
	var buf strings.Builder
	err := generateASCIIQRCode("", &buf)
	if err == nil {
		t.Fatal("expected error for empty string")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Detail.Type != "encode_error" {
		t.Errorf("error type = %q, want %q", exitErr.Detail.Type, "encode_error")
	}
}
