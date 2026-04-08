// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/larksuite/cli/internal/charcheck"
)

// unsafeResourceChars matches URL-special characters, control characters,
// and percent signs (to prevent %2e%2e encoding bypass).
var unsafeResourceChars = regexp.MustCompile(`[?#%\x00-\x1f\x7f]`)

// ResourceName validates an API resource identifier (messageId, fileToken, etc.)
// before it is interpolated into a URL path via fmt.Sprintf. It rejects path
// traversal (..), URL metacharacters (?#%), percent-encoded bypasses (%2e%2e),
// control characters, and dangerous Unicode.
//
// Without this check, an input like "../admin" or "?evil=true" in a message ID
// would alter the API endpoint the request is sent to. Works alongside
// EncodePathSegment for defense-in-depth.
func ResourceName(name, flagName string) error {
	if name == "" {
		return fmt.Errorf("%s must not be empty", flagName)
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return fmt.Errorf("%s must not contain '..' path traversal", flagName)
		}
	}
	if unsafeResourceChars.MatchString(name) {
		return fmt.Errorf("%s contains invalid characters", flagName)
	}
	for _, r := range name {
		if charcheck.IsDangerousUnicode(r) {
			return fmt.Errorf("%s contains dangerous Unicode characters", flagName)
		}
	}
	return nil
}

// EncodePathSegment percent-encodes user input for safe use as a single URL path
// segment (e.g. / → %2F, ? → %3F, # → %23), ensuring the value cannot alter the
// URL routing structure when interpolated into an API path.
//
// This provides defense-in-depth alongside ResourceName: ResourceName rejects known
// dangerous patterns at the input layer, while EncodePathSegment acts as a fallback
// at the concatenation layer — if ResourceName rules are relaxed in the future, or
// if an API path bypasses ResourceName validation (e.g. cmd/service/ generic calls),
// encoding still prevents special characters from being interpreted as path separators
// or query parameters.
//
// Convention: all user-provided variables in fmt.Sprintf API paths within shortcuts/
// MUST be wrapped with this function.
func EncodePathSegment(s string) string {
	return url.PathEscape(s)
}
