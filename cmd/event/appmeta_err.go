// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"fmt"
	"regexp"
)

// authURLPattern matches the "grant-scope" URL the Feishu open platform
// embeds in 99991672-family permission errors. Host allowlist kept in
// sync with consoleScopeGrantURL's output (cmd/event/console_url.go,
// which resolves via core.ResolveEndpoints).Open to feishu.cn or
// larksuite.com). If you add a brand there, widen this regex too.
var authURLPattern = regexp.MustCompile(`https?://open\.(?:feishu\.cn|larksuite\.com)/app/[^/\s"']+/auth\?q=[^\s"'<>]+`)

// describeAppMetaErr reduces an appmeta.FetchCurrentPublished error to a one-
// line stderr-friendly summary. The app_versions OAPI dumps a multi-hundred-
// character JSON body (full msg + troubleshooter + log_id + permission_
// violations) when the bot lacks application-info scopes — useless for
// humans and noisy for AI agents parsing stderr.
//
// Strategy:
//   - If the error contains a /auth?q=... grant URL (the 99991672 shape),
//     emit a short "needs scope X; grant at URL" line.
//   - Otherwise truncate to keep stderr single-screen.
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
