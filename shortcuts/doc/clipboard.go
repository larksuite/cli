// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"
	"os"
	"os/exec"
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
	f, err := os.CreateTemp("", "lark-clipboard-*.png")
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

// readClipboardDarwin uses the built-in osascript to write the clipboard PNG
// to a temp file. No external dependencies required.
func readClipboardDarwin(destPath string) error {
	// AppleScript writes clipboard PNG data directly to a POSIX path.
	script := fmt.Sprintf(
		`set f to open for access POSIX file %q with write permission
write (the clipboard as «class PNGf») to f
close access f`, destPath)

	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("clipboard contains no image data (%s)", msg)
	}
	return nil
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
