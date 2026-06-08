// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package filecheck provides mail attachment file validation utilities shared
// by the emlbuilder and draft packages.
package filecheck

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

// blockedExtensions is the set of file extensions that are not allowed as mail
// attachments. These are potentially harmful executable or script types that
// should be blocked for security reasons (htm/html are intentionally excluded).
var blockedExtensions = map[string]struct{}{
	"action":      {},
	"apk":         {},
	"app":         {},
	"applescript": {},
	"asp":         {},
	"awk":         {},
	"bash":        {},
	"bat":         {},
	"bin":         {},
	"cdxml":       {},
	"chm":         {},
	"cmd":         {},
	"coffee":      {},
	"com":         {},
	"command":     {},
	"cpl":         {},
	"csh":         {},
	"dart":        {},
	"dll":         {},
	"es":          {},
	"exe":         {},
	"fish":        {},
	"gadget":      {},
	"go":          {},
	"hta":         {},
	"inf1":        {},
	"ins":         {},
	"inx":         {},
	"ipa":         {},
	"isu":         {},
	"jar":         {},
	"job":         {},
	"js":          {},
	"jse":         {},
	"ksh":         {},
	"lnk":         {},
	"lua":         {},
	"msc":         {},
	"msh":         {},
	"msh1":        {},
	"msh1xml":     {},
	"msh2":        {},
	"msh2xml":     {},
	"mshxml":      {},
	"msi":         {},
	"msp":         {},
	"mst":         {},
	"msu":         {},
	"osx":         {},
	"out":         {},
	"paf":         {},
	"php":         {},
	"pif":         {},
	"pl":          {},
	"plist":       {},
	"pls":         {},
	"pm":          {},
	"prg":         {},
	"ps":          {},
	"ps1":         {},
	"ps1xml":      {},
	"ps2":         {},
	"ps2xml":      {},
	"psc1":        {},
	"psc2":        {},
	"psd1":        {},
	"psdm1":       {},
	"psm1":        {},
	"pssc":        {},
	"py":          {},
	"pyc":         {},
	"pyo":         {},
	"pyw":         {},
	"pyz":         {},
	"pyzw":        {},
	"rb":          {},
	"reg":         {},
	"rgs":         {},
	"run":         {},
	"scf":         {},
	"scr":         {},
	"sct":         {},
	"sh":          {},
	"shb":         {},
	"shs":         {},
	"tcsh":        {},
	"terminal":    {},
	"ts":          {},
	"tsx":         {},
	"u3p":         {},
	"vb":          {},
	"vbe":         {},
	"vbs":         {},
	"vbscript":    {},
	"ws":          {},
	"wsc":         {},
	"wsf":         {},
	"wsh":         {},
	"zsh":         {},
}

// CheckBlockedExtension returns an error if the filename has a blocked extension.
func CheckBlockedExtension(filename string) error {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	if ext == "" {
		return nil
	}
	if _, ok := blockedExtensions[ext]; ok {
		return fmt.Errorf("file extension %q is not allowed as a mail attachment", "."+ext) //nolint:forbidigo // intermediate mail file-format check; mail command layer wraps into typed ValidationError.
	}
	return nil
}

// allowedInlineExtensions is the whitelist of file extensions allowed for
// inline images. Only well-supported image formats are included.
var allowedInlineExtensions = map[string]struct{}{
	"jpg":  {},
	"jpeg": {},
	"png":  {},
	"gif":  {},
	"webp": {},
}

// allowedInlineMIMETypes is the whitelist of MIME types allowed for inline
// images, checked via content sniffing (http.DetectContentType).
var allowedInlineMIMETypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/gif":  {},
	"image/webp": {},
}

// CheckInlineImageFormat validates that the file is an allowed inline image
// format by checking both extension and content-sniffed MIME type.
// Both must match the whitelist to prevent extension spoofing and MIME forgery.
// On success it returns the detected MIME type; callers MUST use this as the
// final Content-Type instead of trusting any user-supplied or inherited value.
func CheckInlineImageFormat(filename string, content []byte) (string, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	if _, ok := allowedInlineExtensions[ext]; !ok {
		return "", fmt.Errorf("inline image extension %q is not allowed; supported formats: jpg, jpeg, png, gif, webp", ext) //nolint:forbidigo // intermediate mail file-format check; mail command layer wraps into typed ValidationError.
	}
	detected := http.DetectContentType(content)
	// DetectContentType may return params (e.g. "text/plain; charset=utf-8"),
	// strip to the base media type.
	if i := strings.IndexByte(detected, ';'); i != -1 {
		detected = strings.TrimSpace(detected[:i])
	}
	if _, ok := allowedInlineMIMETypes[detected]; !ok {
		return "", fmt.Errorf("inline image content type %q does not match an allowed image format; supported: image/jpeg, image/png, image/gif, image/webp", detected) //nolint:forbidigo // intermediate mail file-format check; mail command layer wraps into typed ValidationError.
	}
	return detected, nil
}
