// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"encoding/base64"
	"os"
	"runtime"
	"strings"
	"testing"
)

// TestReadClipboardImageBytes_EmptyResultReturnsError locks in the contract
// that readClipboardImageBytes surfaces a clear error (instead of silently
// succeeding with empty bytes) whenever the platform layer produced no image
// data. On Linux runners this is exercised by reusing the "no clipboard tool
// found" path, which is the only portable way to force an empty result
// without a display/pasteboard.
func TestReadClipboardImageBytes_EmptyResultReturnsError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("portable empty-result check only runs on Linux; macOS/Windows require a real pasteboard")
	}
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", "")

	data, err := readClipboardImageBytes()
	if err == nil {
		t.Fatalf("expected error on empty clipboard, got data=%d bytes", len(data))
	}
	if len(data) != 0 {
		t.Errorf("expected no data when readClipboardImageBytes errors, got %d bytes", len(data))
	}
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

func TestReadClipboardLinux_XselRejectsNonPNG(t *testing.T) {
	// Fake xsel that returns plain text (non-PNG) — should be rejected by the
	// PNG-magic validation so the user does not upload text as an "image".
	tmpDir := t.TempDir()
	fakeXsel := tmpDir + "/xsel"
	if err := os.WriteFile(fakeXsel, []byte("#!/bin/sh\nprintf 'not a png'\n"), 0755); err != nil {
		t.Fatalf("write fake xsel: %v", err)
	}

	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", tmpDir) // no xclip, no wl-paste; only our fake xsel

	_, err := readClipboardLinux()
	if err == nil {
		t.Fatal("expected error when xsel returns non-PNG bytes, got nil")
	}
}

func TestHasPNGMagic(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want bool
	}{
		{"exact PNG signature", []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, true},
		{"PNG signature plus payload", []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0xde, 0xad}, true},
		{"plain text", []byte("not a png"), false},
		{"empty", []byte{}, false},
		{"too short", []byte{0x89, 0x50, 0x4e, 0x47}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasPNGMagic(tt.in); got != tt.want {
				t.Errorf("hasPNGMagic(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestReadClipboardImageBytes_UnsupportedPlatform(t *testing.T) {
	// The dispatcher returns a clear error on platforms we do not support.
	// We cannot flip runtime.GOOS, but we can cover the shared post-processing
	// by invoking the function on any platform and asserting the non-error
	// contract holds: either it returns data (unlikely in CI) or an error —
	// never both zero values.
	data, err := readClipboardImageBytes()
	if err == nil && len(data) == 0 {
		t.Fatal("readClipboardImageBytes returned (nil, nil); must return error when data is empty")
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

// TestReBase64DataURI_LineWrapped exercises the common real-world case where
// HTML or RTF clipboards fold a base64 payload at 76 chars (standard MIME
// line wrapping). The regex must capture whitespace inside the payload so
// strings.Fields can strip it before base64 decoding; otherwise the match is
// truncated at the first newline and the decoded prefix happens to pass
// hasKnownImageMagic (since PNG magic is just 8 bytes), silently uploading a
// corrupt payload.
func TestReBase64DataURI_LineWrapped(t *testing.T) {
	// Build a deterministic payload larger than one wrap line so we force a
	// fold. The exact bytes don't matter; the full round-trip does.
	payload := make([]byte, 180)
	for i := range payload {
		payload[i] = byte(i * 7)
	}
	b64 := base64.StdEncoding.EncodeToString(payload)

	// Insert realistic folding: a mix of \n, \r\n, and \t within a single
	// payload, to catch regressions regardless of the clipboard source
	// (HTML tends to use \n; RTF \par wraps use \r\n; some editors indent).
	if len(b64) < 120 {
		t.Fatalf("test payload too small for folding: len=%d", len(b64))
	}
	wrapped := b64[:40] + "\n   " + b64[40:80] + "\r\n\t" + b64[80:]
	html := `<img src="data:image/png;base64,` + wrapped + `">`

	m := reBase64DataURI.FindSubmatch([]byte(html))
	if m == nil {
		t.Fatal("expected regex to match line-wrapped base64 payload")
	}
	if string(m[1]) != "image/png" {
		t.Errorf("mime type = %q, want %q", m[1], "image/png")
	}

	// The whole point of extending the character class: the downstream
	// Fields strip must see the folding and normalise it away.
	normalized := strings.Join(strings.Fields(string(m[2])), "")
	if normalized != b64 {
		t.Fatalf("normalized payload mismatch\n got: %q\nwant: %q", normalized, b64)
	}
	got, err := base64.StdEncoding.DecodeString(normalized)
	if err != nil {
		t.Fatalf("decode after normalisation failed: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Error("decoded bytes differ from original payload — truncation regression")
	}

	// The match must still stop at the URI boundary; extending the class
	// with \s should not let the capture run off the end of the attribute.
	if strings.Contains(string(m[0]), `">`) {
		t.Errorf("regex captured past the URI terminator: %q", m[0])
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
