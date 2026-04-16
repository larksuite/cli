// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/larksuite/cli/internal/vfs"
)

// readClipboardImageBytes reads the current clipboard image and returns the
// raw PNG bytes in memory. No temporary files are created by the caller;
// any intermediate files required by platform tools (e.g. sips on macOS) are
// created via vfs and cleaned up before returning.
//
// Platform support:
//
//	macOS   — osascript (built-in, no extra deps); sips for TIFF→PNG conversion
//	Windows — powershell + System.Windows.Forms (built-in), output as base64
//	Linux   — xclip (X11), wl-paste (Wayland), or xsel (X11 fallback),
//	          tried in that order; returns a clear error if none is found.
func readClipboardImageBytes() ([]byte, error) {
	var data []byte
	var err error

	switch runtime.GOOS {
	case "darwin":
		data, err = readClipboardDarwin()
	case "windows":
		data, err = readClipboardWindows()
	case "linux":
		data, err = readClipboardLinux()
	default:
		return nil, fmt.Errorf("clipboard image upload is not supported on %s", runtime.GOOS)
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("clipboard contains no image data")
	}
	return data, nil
}

// reBase64DataURI matches a data URI image embedded in clipboard text content,
// e.g. data:image/jpeg;base64,/9j/4AAQ...
// The character class covers both standard (+/) and URL-safe (-_) base64 alphabets.
var reBase64DataURI = regexp.MustCompile(`data:(image/[^;]+);base64,([A-Za-z0-9+/\-_]+=*)`)

// readClipboardDarwin reads the clipboard image on macOS and returns PNG bytes.
//
// Strategy:
//  1. Ask osascript for the clipboard as PNG (hex literal on stdout) → decode.
//  2. Ask osascript for the clipboard as TIFF (hex literal on stdout) → decode →
//     convert to PNG with sips (built-in macOS tool) via vfs temp files.
//  3. Scan all text-based clipboard formats (HTML, RTF, plain text) for an
//     embedded base64 data URI image (e.g. images copied from Feishu / browsers).
//
// No external dependencies required — osascript and sips ship with macOS.
func readClipboardDarwin() ([]byte, error) {
	// Attempt 1: PNG via osascript hex literal on stdout.
	out, err := exec.Command("osascript", "-e", "get the clipboard as «class PNGf»").CombinedOutput()
	if err == nil && len(out) > 0 {
		if data, decErr := decodeOsascriptData(strings.TrimSpace(string(out))); decErr == nil && len(data) > 0 {
			return data, nil
		}
	}

	// Attempt 2: TIFF via osascript hex literal → decode → convert to PNG with sips.
	out, err = exec.Command("osascript", "-e", "get the clipboard as «class TIFF»").CombinedOutput()
	if err == nil && len(out) > 0 {
		tiffData, decErr := decodeOsascriptData(strings.TrimSpace(string(out)))
		if decErr == nil && len(tiffData) > 0 {
			if pngData, convErr := convertTIFFToPNGViaSips(tiffData); convErr == nil {
				return pngData, nil
			}
		}
	}

	// Attempt 3: scan text-based clipboard formats for an embedded base64 data URI.
	// Covers HTML (Feishu, Chrome, Safari), RTF, and plain text — tried in order.
	if imgData := extractBase64ImageFromClipboard(); imgData != nil {
		return imgData, nil
	}

	return nil, fmt.Errorf("clipboard contains no image data")
}

// convertTIFFToPNGViaSips writes tiffData to a vfs temp file, runs sips to
// convert it to PNG in a second temp file, reads the result, and cleans up.
func convertTIFFToPNGViaSips(tiffData []byte) ([]byte, error) {
	tiffFile, err := vfs.CreateTemp("", "lark-clip-*.tiff")
	if err != nil {
		return nil, fmt.Errorf("clipboard: create tiff temp: %w", err)
	}
	tiffPath := tiffFile.Name()
	tiffFile.Close()
	defer vfs.Remove(tiffPath) //nolint:errcheck

	if err = vfs.WriteFile(tiffPath, tiffData, 0600); err != nil {
		return nil, fmt.Errorf("clipboard: write tiff temp: %w", err)
	}

	pngFile, err := vfs.CreateTemp("", "lark-clip-*.png")
	if err != nil {
		return nil, fmt.Errorf("clipboard: create png temp: %w", err)
	}
	pngPath := pngFile.Name()
	pngFile.Close()
	defer vfs.Remove(pngPath) //nolint:errcheck

	if out, sipsErr := exec.Command("sips", "-s", "format", "png", tiffPath, "--out", pngPath).CombinedOutput(); sipsErr != nil {
		msg := strings.TrimSpace(string(out))
		return nil, fmt.Errorf("clipboard image conversion failed (sips: %s)", msg)
	}

	return vfs.ReadFile(pngPath)
}

// clipboardTextFormats lists the osascript type coercions to try when looking
// for an embedded base64 data-URI image in text-based clipboard formats.
// Ordered by likelihood of containing an embedded image.
var clipboardTextFormats = []struct {
	classCode string // 4-char OSType used in «class XXXX»
	asExpr    string // AppleScript coercion expression
}{
	{"HTML", "get the clipboard as «class HTML»"},
	{"RTF ", "get the clipboard as «class RTF »"},
	{"utf8", "get the clipboard as «class utf8»"},
	{"TEXT", "get the clipboard as string"},
}

// extractBase64ImageFromClipboard iterates text clipboard formats and returns
// the first decoded image payload found, or nil if none contains image data.
func extractBase64ImageFromClipboard() []byte {
	for _, f := range clipboardTextFormats {
		out, err := exec.Command("osascript", "-e", f.asExpr).CombinedOutput()
		if err != nil || len(out) == 0 {
			continue
		}
		raw := strings.TrimSpace(string(out))
		decoded, err := decodeOsascriptData(raw)
		if err != nil || len(decoded) == 0 {
			continue
		}
		m := reBase64DataURI.FindSubmatch(decoded)
		if m == nil {
			continue
		}
		// Accept both standard and URL-safe base64 (some apps emit URL-safe).
		imgData, err := base64.StdEncoding.DecodeString(string(m[2]))
		if err != nil {
			imgData, err = base64.URLEncoding.DecodeString(string(m[2]))
		}
		if err == nil && len(imgData) > 0 {
			return imgData
		}
	}
	return nil
}

// decodeOsascriptData converts the «data XXXX<hex>» literal that osascript
// emits for binary clipboard classes into raw bytes.
// If the input does not match the literal format, the raw bytes are returned as-is.
func decodeOsascriptData(s string) ([]byte, error) {
	// Format: «data HTML3C6D657461...»
	const prefix = "\xc2\xab" + "data " // « in UTF-8 followed by "data "
	if !strings.HasPrefix(s, prefix) {
		// plain string — return as-is
		return []byte(s), nil
	}
	// strip «data XXXX (4-char class code follows immediately, no space) and trailing »
	s = s[len(prefix):]
	if len(s) >= 4 {
		s = s[4:] // skip class code, e.g. "HTML", "TIFF", "PNGf"
	}
	s = strings.TrimSuffix(s, "\xc2\xbb") // »
	s = strings.TrimSpace(s)
	return decodeHex(s)
}

// decodeHex decodes an uppercase hex string (as produced by osascript) to bytes.
func decodeHex(h string) ([]byte, error) {
	if len(h)%2 != 0 {
		return nil, fmt.Errorf("odd hex length")
	}
	b := make([]byte, len(h)/2)
	for i := 0; i < len(h); i += 2 {
		hi := hexVal(h[i])
		lo := hexVal(h[i+1])
		if hi < 0 || lo < 0 {
			return nil, fmt.Errorf("invalid hex char at %d", i)
		}
		b[i/2] = byte(hi<<4 | lo)
	}
	return b, nil
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// readClipboardWindows uses PowerShell to export the clipboard image as PNG,
// writing it as base64 to stdout and decoding in Go (no temp files).
func readClipboardWindows() ([]byte, error) {
	script := `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$img = [System.Windows.Forms.Clipboard]::GetImage()
if ($img -eq $null) { Write-Error 'clipboard contains no image data'; exit 1 }
$ms = New-Object System.IO.MemoryStream
$img.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
[Convert]::ToBase64String($ms.ToArray())
`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("clipboard read failed (%s)", msg)
	}
	b64 := strings.TrimSpace(string(out))
	data, decErr := base64.StdEncoding.DecodeString(b64)
	if decErr != nil {
		return nil, fmt.Errorf("clipboard image decode failed: %w", decErr)
	}
	return data, nil
}

// pngMagic is the 8-byte PNG signature used to validate clipboard output from
// tools that cannot negotiate MIME types (e.g. xsel).
var pngMagic = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

func hasPNGMagic(b []byte) bool {
	return len(b) >= len(pngMagic) && string(b[:len(pngMagic)]) == string(pngMagic)
}

// readClipboardLinux tries xclip (X11), wl-paste (Wayland), and xsel (X11)
// in order, returning the PNG bytes from the first available tool.
//
// xclip and wl-paste request the image/png MIME type directly; xsel cannot
// negotiate MIME types so its output is validated against the PNG magic header.
// If a tool is present but fails or returns non-PNG data, the error is
// preserved so users see a meaningful message instead of "no tool found".
func readClipboardLinux() ([]byte, error) {
	type tool struct {
		name        string
		args        []string
		validatePNG bool // true when the tool cannot request image/png by MIME
	}
	tools := []tool{
		{"xclip", []string{"-selection", "clipboard", "-t", "image/png", "-o"}, false},
		{"wl-paste", []string{"--type", "image/png"}, false},
		{"xsel", []string{"--clipboard", "--output"}, true},
	}

	var lastErr error
	foundTool := false
	for _, t := range tools {
		if _, lookErr := exec.LookPath(t.name); lookErr != nil {
			continue
		}
		foundTool = true
		out, err := exec.Command(t.name, t.args...).Output()
		if err != nil {
			lastErr = fmt.Errorf("clipboard image read failed via %s: %w", t.name, err)
			continue
		}
		if len(out) == 0 {
			lastErr = fmt.Errorf("clipboard contains no image data (%s returned empty output)", t.name)
			continue
		}
		if t.validatePNG && !hasPNGMagic(out) {
			lastErr = fmt.Errorf("clipboard contains no PNG image data (%s output is not a PNG)", t.name)
			continue
		}
		return out, nil
	}

	if foundTool && lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf(
		"clipboard image read failed: no supported tool found\n" +
			"  X11:    sudo apt install xclip   (or: sudo yum install xclip)\n" +
			"  Wayland: sudo apt install wl-clipboard")
}
