// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// readClipboardToTempFile reads the current clipboard image and saves it to a
// temporary PNG file. The caller must call the returned cleanup function to
// remove the temp file when done, regardless of any subsequent errors.
//
// Platform support:
//
//	macOS   — osascript (built-in, no extra deps)
//	Windows — powershell + System.Windows.Forms (built-in)
//	Linux   — xclip (X11), wl-paste (Wayland), or xsel (X11 fallback),
//	          tried in that order; returns a clear error if none is found.
func readClipboardToTempFile() (path string, cleanup func(), err error) {
	// Create the temp file in the current directory so it passes the FileIO
	// relative-path validation used by the upload pipeline.
	f, err := os.CreateTemp(".", "lark-clipboard-*.png")
	if err != nil {
		return "", func() {}, fmt.Errorf("clipboard: create temp file: %w", err)
	}
	path = f.Name()
	f.Close()

	cleanup = func() { os.Remove(path) }

	switch runtime.GOOS {
	case "darwin":
		err = readClipboardDarwin(path)
	case "windows":
		err = readClipboardWindows(path)
	case "linux":
		err = readClipboardLinux(path)
	default:
		err = fmt.Errorf("clipboard image upload is not supported on %s", runtime.GOOS)
	}

	if err != nil {
		cleanup()
		return "", func() {}, err
	}

	// Verify the file has content (empty = no image in clipboard)
	info, statErr := os.Stat(path)
	if statErr != nil || info.Size() == 0 {
		cleanup()
		return "", func() {}, fmt.Errorf("clipboard contains no image data")
	}

	return path, cleanup, nil
}

// reBase64DataURI matches a data URI image embedded in HTML clipboard content,
// e.g. data:image/jpeg;base64,/9j/4AAQ...
var reBase64DataURI = regexp.MustCompile(`data:(image/[^;]+);base64,([A-Za-z0-9+/]+=*)`)

// readClipboardDarwin reads the clipboard image on macOS.
//
// Strategy:
//  1. Try to coerce the clipboard to PNG via osascript.
//  2. If that fails (e.g. screenshot is stored as TIFF), fall back to TIFF,
//     then convert to PNG using sips (Scriptable Image Processing System),
//     which is a macOS built-in at /usr/bin/sips.
//  3. If neither native image format is present, try to extract a base64-encoded
//     image from the HTML clipboard (e.g. images copied from Feishu / browsers).
//
// No external dependencies required — osascript and sips ship with macOS.
func readClipboardDarwin(destPath string) error {
	// Attempt 1: PNG (works when image was copied from browser / app)
	pngScript := fmt.Sprintf(
		`set f to open for access POSIX file %q with write permission
write (the clipboard as «class PNGf») to f
close access f`, destPath)
	if out, err := exec.Command("osascript", "-e", pngScript).CombinedOutput(); err == nil {
		_ = out
		return nil
	}

	// Attempt 2: TIFF (default for macOS screenshots) → convert to PNG via sips
	tiffPath := destPath + ".tiff"
	tiffScript := fmt.Sprintf(
		`set f to open for access POSIX file %q with write permission
write (the clipboard as «class TIFF») to f
close access f`, tiffPath)
	if out, err := exec.Command("osascript", "-e", tiffScript).CombinedOutput(); err == nil {
		_ = out
		defer os.Remove(tiffPath)
		// Convert TIFF → PNG using sips (built-in macOS tool)
		if out2, err2 := exec.Command("sips", "-s", "format", "png", tiffPath, "--out", destPath).CombinedOutput(); err2 != nil {
			msg := strings.TrimSpace(string(out2))
			return fmt.Errorf("clipboard image conversion failed (sips: %s)", msg)
		}
		return nil
	}

	// Attempt 3: HTML clipboard with embedded base64 data URI
	// (e.g. images copied from Feishu docs, Chrome, Safari)
	htmlOut, err := exec.Command("osascript", "-e", "get the clipboard as «class HTML»").CombinedOutput()
	if err != nil {
		return fmt.Errorf("clipboard contains no image data")
	}
	// osascript returns the raw bytes as a hex «data HTML...» literal; decode it.
	raw := strings.TrimSpace(string(htmlOut))
	htmlBytes, err := decodeOsascriptData(raw)
	if err != nil || len(htmlBytes) == 0 {
		return fmt.Errorf("clipboard contains no image data")
	}
	m := reBase64DataURI.FindSubmatch(htmlBytes)
	if m == nil {
		return fmt.Errorf("clipboard contains no image data (HTML clipboard has no embedded image)")
	}
	imgData, err := base64.StdEncoding.DecodeString(string(m[2]))
	if err != nil {
		return fmt.Errorf("clipboard image decode failed: %w", err)
	}
	return os.WriteFile(destPath, imgData, 0600)
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

// readClipboardWindows uses PowerShell's System.Windows.Forms.Clipboard
// (built-in on all modern Windows) to export the clipboard image as PNG.
func readClipboardWindows(destPath string) error {
	// Single-quoted path avoids most escaping issues; backslashes are fine here.
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$img = [System.Windows.Forms.Clipboard]::GetImage()
if ($img -eq $null) { Write-Error 'clipboard contains no image data'; exit 1 }
$img.Save('%s', [System.Drawing.Imaging.ImageFormat]::Png)
`, destPath)

	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("clipboard read failed (%s)", msg)
	}
	return nil
}

// readClipboardLinux tries xclip (X11), wl-paste (Wayland), and xsel (X11)
// in order, using the first available tool.
func readClipboardLinux(destPath string) error {
	type tool struct {
		name string
		args []string
	}
	tools := []tool{
		{"xclip", []string{"-selection", "clipboard", "-t", "image/png", "-o"}},
		{"wl-paste", []string{"--type", "image/png"}},
		{"xsel", []string{"--clipboard", "--output"}},
	}

	for _, t := range tools {
		if _, lookErr := exec.LookPath(t.name); lookErr != nil {
			continue
		}
		out, err := exec.Command(t.name, t.args...).Output()
		if err != nil || len(out) == 0 {
			return fmt.Errorf("clipboard contains no image data (%s returned empty output)", t.name)
		}
		return os.WriteFile(destPath, out, 0600)
	}

	return fmt.Errorf(
		"clipboard image read failed: no supported tool found\n" +
			"  X11:    sudo apt install xclip   (or: sudo yum install xclip)\n" +
			"  Wayland: sudo apt install wl-clipboard")
}
