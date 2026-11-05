// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
)

// rejectDriveMiaodaURL keeps app-ID lookup in the agent workflow. Only Miaoda's
// /app/app_... shape is recognized: other /app/ links may belong to BaseApp.
// Do not interpolate URL contents into the recovery command.
func rejectDriveMiaodaURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	// This is a recognizer, not a general token validator. Leave malformed or
	// unrelated input to the calling command's existing target validation.
	if err == nil && (parsed.Scheme == "" || parsed.Scheme == "http" || parsed.Scheme == "https") &&
		strings.HasPrefix(parsed.Path, "/app/app_") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--token requires a meta_token, not a Miaoda app URL").
			WithParam("--token").
			WithHint("extract app_id from /app/<app_id> and run `lark-cli apps +get --app-id <app_id> --as user`; then retry this command with --token set to data.app.meta_token and --type set to data.app.meta_doc_type (apps or miaoda_apps). If either field is missing, stop; do not guess the token or type")
	}
	return nil
}
