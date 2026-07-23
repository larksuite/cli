// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package localfileio

import (
	"fmt"
	"path/filepath"
	"strings"
)

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
