// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// readClipboardImageBytes reads the current clipboard image and returns the
// raw PNG bytes in memory. No temporary files are created on any platform;
// all platform tools emit image bytes (or an encoded form) on stdout.
//
// Platform support:
//
//	macOS   — osascript (built-in, no extra deps)
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
// The character class covers both standard (+/) and URL-safe (-_) base64
// alphabets, plus ASCII whitespace: HTML and RTF clipboard payloads commonly
// fold long base64 at 76 chars (standard MIME folding), so whitespace must be
// captured as part of the payload for the downstream strings.Fields strip to
// actually have something to normalise. Terminators like ", <, ), ; remain
// outside the class so the match still ends at the URI boundary.
var reBase64DataURI = regexp.MustCompile(`data:(image/[^;]+);base64,([A-Za-z0-9+/\-_\s]+=*)`)

// readClipboardDarwin reads the clipboard image on macOS and returns image bytes.
//
// Strategy:
//  1. Ask osascript for the clipboard as PNG (hex literal on stdout) → decode.
//     Native macOS screenshots and most image-producing apps place PNG on the
//     pasteboard directly.
//  2. Scan all text-based clipboard formats (HTML, RTF, plain text) for an
//     embedded base64 data URI image (e.g. images copied from Feishu / browsers).
//     Decoded payload is validated against known image magic bytes so text
//     clipboards that happen to mention a data URI literally are not treated
//     as image data.
//
// No external dependencies required — osascript ships with macOS.
func readClipboardDarwin() ([]byte, error) {
	// Attempt 1: PNG via osascript hex literal on stdout.
	// Use Output() + separate stderr capture so osascript diagnostics
	// (locale warnings, AppleEvent permission prompts, etc.) do not
	// contaminate the decoded payload or mask real failures.
	out, stderrText, runErr := runOsascript("get the clipboard as «class PNGf»")
	if runErr == nil && len(out) > 0 {
		if data, decErr := decodeOsascriptData(strings.TrimSpace(string(out))); decErr == nil && len(data) > 0 {
			return data, nil
		}
	}
	// First-attempt failure is expected for non-image clipboards — fall through
	// to the base64 scan. Keep the stderr text for the final error message in
	// case every attempt ends up empty-handed.

	// Attempt 2: scan text-based clipboard formats for an embedded base64 data URI.
	// Covers HTML (Feishu, Chrome, Safari), RTF, and plain text — tried in order.
	if imgData := extractBase64ImageFromClipboard(); imgData != nil {
		return imgData, nil
	}

	if stderrText != "" {
		return nil, fmt.Errorf("clipboard contains no image data (osascript: %s)", stderrText)
	}
	return nil, fmt.Errorf("clipboard contains no image data")
}

// runOsascript invokes osascript with a single AppleScript expression and
// returns stdout, a trimmed stderr string, and the exec error separately.
// Using Output() (rather than CombinedOutput) keeps stderr out of the decoded
// payload, while the captured stderr is still available for error messages.
func runOsascript(expr string) (stdout []byte, stderrText string, err error) {
	cmd := exec.Command("osascript", "-e", expr)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err = cmd.Output()
	stderrText = strings.TrimSpace(stderr.String())
	return stdout, stderrText, err
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
// Decoded bytes are validated against known image magic headers so that
// text clipboards containing a literal `data:image/...;base64,...` fragment
// (e.g. a tutorial, a code sample, pasted HTML source) are not silently
// uploaded as an image.
func extractBase64ImageFromClipboard() []byte {
	for _, f := range clipboardTextFormats {
		out, _, err := runOsascript(f.asExpr)
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
		// HTML/RTF clipboard content often line-wraps base64 at 76 chars; strip
		// all ASCII whitespace before decoding so wrapped payloads are not missed.
		// Accept both standard and URL-safe base64 (some apps emit URL-safe).
		b64 := strings.Join(strings.Fields(string(m[2])), "")
		imgData, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			imgData, err = base64.URLEncoding.DecodeString(b64)
		}
		if err != nil || len(imgData) == 0 {
			continue
		}
		if !hasKnownImageMagic(imgData) {
			// Decoded payload does not look like a real image — e.g. the
			// clipboard is a documentation sample that mentions data URIs.
			// Keep looking in the next format rather than upload garbage.
			continue
		}
		return imgData
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
	// Use Output() + captured stderr so PowerShell diagnostics surface in the
	// error message but never corrupt the base64 stdout we need to decode.
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
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

// imageMagics enumerates the leading-byte signatures we accept as "this is a
// real image payload" when a text clipboard supplies a base64 data URI. The
// set mirrors the formats the Lark upload endpoints already accept; other
// rare formats fall through so the caller skips to the next clipboard format.
var imageMagics = [][]byte{
	// PNG
	{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a},
	// JPEG (SOI)
	{0xff, 0xd8, 0xff},
	// GIF87a / GIF89a
	[]byte("GIF87a"),
	[]byte("GIF89a"),
	// WebP: "RIFF????WEBP" — check the RIFF marker only; the WEBP marker
	// lives at offset 8, validated separately below.
	[]byte("RIFF"),
	// BMP
	[]byte("BM"),
}

// hasKnownImageMagic reports whether the first bytes of b match any of the
// image signatures we trust. RIFF is further constrained to actual WebP
// streams to avoid false positives on other RIFF-based formats (WAV, AVI).
func hasKnownImageMagic(b []byte) bool {
	for _, magic := range imageMagics {
		if len(b) < len(magic) {
			continue
		}
		if string(b[:len(magic)]) != string(magic) {
			continue
		}
		// RIFF header must be followed at offset 8 by "WEBP" to count as an image.
		if string(magic) == "RIFF" {
			if len(b) >= 12 && string(b[8:12]) == "WEBP" {
				return true
			}
			continue
		}
		return true
	}
	return false
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
		"clipboard image read failed: no supported tool found. " +
			"Install one of xclip, wl-clipboard, or xsel via your distro's package manager " +
			"(apt, dnf, pacman, apk, brew, etc.).")
}
