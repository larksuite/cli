// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package argstype

import (
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// MediaInput is the tri-state media-field value used by im image/file/video/
// audio flags: URL, "img_xxx"/"file_xxx" key, or cwd-relative local path.
// URL and key forms bypass path safety checks; local paths go through the
// same SafePath rules. Does not emit absolute paths in hints (log safety).
type MediaInput string

// IsURL reports whether the value looks like an http(s) URL.
func (m MediaInput) IsURL() bool {
	s := string(m)
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// IsMediaKey reports whether the value is an already-uploaded media key.
func (m MediaInput) IsMediaKey() bool {
	s := string(m)
	return strings.HasPrefix(s, "img_") || strings.HasPrefix(s, "file_")
}

func (m MediaInput) ValidateValue(rt *common.RuntimeContext, flagName string) error {
	if string(m) == "" {
		return nil
	}
	if m.IsURL() || m.IsMediaKey() {
		return nil
	}
	return SafePath(m).ValidateValue(rt, flagName)
}
