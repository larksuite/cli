// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"fmt"
	"regexp"
)

// authURLPattern matches the grant-scope URL embedded in 99991672 errors; widen when adding brands in consoleScopeGrantURL.
var authURLPattern = regexp.MustCompile(`https?://open\.(?:feishu\.cn|larksuite\.com)/app/[^/\s"']+/auth\?q=[^\s"'<>]+`)

// describeAppMetaErr reduces a FetchCurrentPublished error to a one-line stderr summary.
func describeAppMetaErr(err error) string {
	msg := err.Error()
	if url := authURLPattern.FindString(msg); url != "" {
		return fmt.Sprintf("bot is missing scopes needed for app-version metadata; grant at: %s", url)
	}
	const maxErrLen = 200
	if len(msg) > maxErrLen {
		return msg[:maxErrLen] + "…"
	}
	return msg
}
