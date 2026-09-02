// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package localfileio

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validatePathPlatform rejects Windows path shapes the policy cannot reason
// about: network/device namespaces (UNC, \\?\) and NTFS alternate data
// streams (a colon anywhere past the drive letter would address a hidden
// stream on an otherwise-allowed file).
func validatePathPlatform(path string) error {
	if isWindowsNonLocalNamespace(path) {
		return fmt.Errorf("path must not use a Windows network or device namespace")
	}
	cleaned := filepath.Clean(path)
	// A drive-relative path ("C:foo") carries a volume but is not absolute: it
	// resolves against that drive's own current directory, so the location it
	// names is not the one this validation can see. It is also how the stream
	// check below would be slipped, since "C:" is stripped as the volume and
	// the remaining "foo" holds no colon.
	if filepath.VolumeName(cleaned) != "" && !filepath.IsAbs(cleaned) {
		return fmt.Errorf("path must not be drive-relative; give a full path or a path without a drive letter")
	}
	if strings.Contains(cleaned[len(filepath.VolumeName(cleaned)):], ":") {
		return fmt.Errorf("path must not address an NTFS alternate data stream")
	}
	return nil
}

func validateLocalInputPlatform(path string) error {
	if isWindowsNonLocalNamespace(path) {
		return fmt.Errorf("local input path must not use a Windows network or device namespace")
	}

	cleaned := filepath.Clean(path)
	volume := filepath.VolumeName(cleaned)
	remainder := strings.TrimLeft(cleaned[len(volume):], `\/`)
	for _, component := range strings.FieldsFunc(remainder, func(r rune) bool {
		return r == '\\' || r == '/'
	}) {
		if component == "." || component == ".." {
			continue
		}
		if !filepath.IsLocal(component) {
			return fmt.Errorf("local input path contains a reserved Windows path component %q", component)
		}
	}
	return nil
}
