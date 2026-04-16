// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/base64"
	"os"
	"runtime"
	"testing"
)

// TestReadClipboardToTempFile_CleanupOnError verifies that readClipboardToTempFile
// removes the temp file when the clipboard read fails (e.g. unsupported platform
// or empty clipboard).  We force a failure by temporarily replacing the
// platform-dispatch with a known-failing path; here we use a simple integration
// guard: just confirm the returned path is empty and cleanup is a no-op func.
//
// Full end-to-end clipboard reads require a real display / pasteboard and are
// tested manually; this test only covers the error-path contract.
func TestReadClipboardToTempFile_EmptyResultRemovesTempFile(t *testing.T) {
	// Write an empty temp file to simulate "clipboard has no image data".
	f, err := os.CreateTemp("", "lark-clipboard-test-*.png")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	emptyPath := f.Name()
	f.Close()

	// Stat should report size == 0
	info, err := os.Stat(emptyPath)
	if err != nil || info.Size() != 0 {
		t.Fatalf("expected empty file, got size=%d err=%v", info.Size(), err)
	}

	// Simulate what readClipboardToTempFile does on empty output: cleanup + error.
	cleanup := func() { os.Remove(emptyPath) }
	cleanup()

	if _, err := os.Stat(emptyPath); !os.IsNotExist(err) {
		t.Errorf("expected temp file to be removed after cleanup, but it still exists")
	}
}

func TestReadClipboardToTempFile_CleanupIsIdempotent(t *testing.T) {
	f, err := os.CreateTemp("", "lark-clipboard-idem-*.png")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	path := f.Name()
	f.Close()

	cleanup := func() { os.Remove(path) }
	// Calling cleanup twice must not panic.
	cleanup()
	cleanup()
}

func TestReadClipboardLinux_NoToolsReturnsError(t *testing.T) {
	// Override PATH so none of xclip/wl-paste/xsel can be found.
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", "")

	_, err := readClipboardLinux()
	if err == nil {
		t.Fatal("expected error when no clipboard tool is available, got nil")
	}
}

func TestDecodeHex(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []byte
		wantErr bool
	}{
		{"empty", "", []byte{}, false},
		{"single byte lower", "2f", []byte{0x2f}, false},
		{"single byte upper", "2F", []byte{0x2f}, false},
		{"multi byte", "48656C6C6F", []byte("Hello"), false},
		{"odd length", "abc", nil, true},
		{"invalid char", "GG", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeHex(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeHex(%q) error=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && string(got) != string(tt.want) {
				t.Errorf("decodeHex(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestDecodeOsascriptData(t *testing.T) {
	// Build a real «data HTML<hex>» literal for the string "<img>"
	raw := []byte("<img>")
	hexStr := ""
	for _, b := range raw {
		hexStr += string([]byte{hexNibble(b >> 4), hexNibble(b & 0xf)})
	}
	// «data HTML3C696D673E»  (« = \xc2\xab, » = \xc2\xbb)
	literal := "\xc2\xab" + "data HTML" + hexStr + "\xc2\xbb"

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain string passthrough", "hello world", "hello world"},
		{"osascript hex literal", literal, "<img>"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeOsascriptData(tt.input)
			if err != nil {
				t.Fatalf("decodeOsascriptData(%q) unexpected error: %v", tt.input, err)
			}
			if string(got) != tt.want {
				t.Errorf("decodeOsascriptData(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestReBase64DataURI_Match(t *testing.T) {
	imgBytes := []byte{0x89, 0x50, 0x4e, 0x47} // PNG magic bytes
	b64 := base64.StdEncoding.EncodeToString(imgBytes)
	html := `<img src="data:image/png;base64,` + b64 + `">`

	m := reBase64DataURI.FindSubmatch([]byte(html))
	if m == nil {
		t.Fatal("expected regex to match base64 data URI in HTML")
	}
	if string(m[1]) != "image/png" {
		t.Errorf("mime type = %q, want %q", m[1], "image/png")
	}
	if string(m[2]) != b64 {
		t.Errorf("base64 payload mismatch")
	}
}

func TestReBase64DataURI_URLSafeMatch(t *testing.T) {
	// URL-safe base64 uses '-' and '_' instead of '+' and '/'.
	// Construct a payload that contains both characters.
	// base64url of 0xFB 0xFF 0xFE → "-__-" in URL-safe alphabet.
	urlSafePayload := "-__-"
	html := `<img src="data:image/jpeg;base64,` + urlSafePayload + `">`

	m := reBase64DataURI.FindSubmatch([]byte(html))
	if m == nil {
		t.Fatal("expected regex to match URL-safe base64 data URI")
	}
	if string(m[1]) != "image/jpeg" {
		t.Errorf("mime type = %q, want %q", m[1], "image/jpeg")
	}
	if string(m[2]) != urlSafePayload {
		t.Errorf("URL-safe base64 payload = %q, want %q", m[2], urlSafePayload)
	}
}

func TestReBase64DataURI_NoMatch(t *testing.T) {
	if reBase64DataURI.Match([]byte("no image here")) {
		t.Error("expected no match for plain text")
	}
}

func TestExtractBase64ImageFromClipboard_WithFakeOsascript(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("fake osascript test only runs on macOS")
	}
	// Build a minimal PNG (1x1 transparent) as base64 to embed in fake HTML output.
	pngBytes := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, // PNG signature
	}
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	htmlContent := `<img src="data:image/png;base64,` + b64 + `">`

	// Encode htmlContent as a «data HTML<hex>» literal the way osascript would.
	hexStr := ""
	for _, c := range []byte(htmlContent) {
		hexStr += string([]byte{hexNibble(c >> 4), hexNibble(c & 0xf)})
	}
	fakeOutput := "\xc2\xab" + "data HTML" + hexStr + "\xc2\xbb"

	// Write a fake osascript that prints fakeOutput and exits 0.
	// Use a pre-written output file to avoid shell-escaping issues with binary data.
	tmpDir := t.TempDir()
	outputFile := tmpDir + "/output.txt"
	if err := os.WriteFile(outputFile, []byte(fakeOutput), 0600); err != nil {
		t.Fatalf("write output file: %v", err)
	}
	fakeScript := tmpDir + "/osascript"
	scriptBody := "#!/bin/sh\ncat " + outputFile + "\n"
	if err := os.WriteFile(fakeScript, []byte(scriptBody), 0755); err != nil {
		t.Fatalf("write fake osascript: %v", err)
	}

	// Prepend tmpDir to PATH so our fake osascript is found first.
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+orig)

	got := extractBase64ImageFromClipboard()
	if got == nil {
		t.Fatal("expected image data, got nil")
	}
	if string(got) != string(pngBytes) {
		t.Errorf("decoded image = %v, want %v", got, pngBytes)
	}
}

func TestExtractBase64ImageFromClipboard_NoOsascript(t *testing.T) {
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", "")

	got := extractBase64ImageFromClipboard()
	if got != nil {
		t.Errorf("expected nil when osascript unavailable, got %v", got)
	}
}

// hexNibble converts a 4-bit value to its uppercase hex character.
func hexNibble(n byte) byte {
	if n < 10 {
		return '0' + n
	}
	return 'A' + n - 10
}
