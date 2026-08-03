// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
)

// appsService 是 CLI 命令的 service 前缀（lark-cli apps ...）。
const appsService = "apps"

// apiBasePath is the registered OAPI prefix for the apps domain.
const apiBasePath = "/open-apis/spark/v1"

// appIDListHint is the shared recovery hint for commands whose most likely
// failure cause is a wrong/inaccessible --app-id. It points at +list to find
// the correct app id. The app_/cli_ format rule is taught in
// lark-apps SKILL.md ("app_id 获取"); the hint stays lean and does not repeat it.
const appIDListHint = "verify --app-id is correct and you have access to the app; list your apps with `lark-cli apps +list`"

// appNoDatabaseCode is the Spark business code returned when a db command runs
// against an app that has not initialized a database yet. The raw server
// message for this code carries internal workspace terminology, so the CLI
// rewrites it into a user-facing explanation and attaches a recoverable
// cloud-development next step (see appNoDatabaseMessage / appNoDatabaseHint).
// The numeric code — not the message text — is the stable discriminator an
// agent harness keys on to enter the recovery flow.
const appNoDatabaseCode = 500002759

// appNoDatabaseMessage is the user-facing explanation for appNoDatabaseCode.
// It deliberately drops internal workspace / db-branch terms.
const appNoDatabaseMessage = "this app does not have a database yet"

// appNoDatabaseHint guides adding a database via Miaoda cloud development. It
// only uses existing commands with stable placeholder args, so a harness can
// execute it without matching natural-language error text. Adding a database is
// a cloud write: a failed read alone does not authorize it — confirm with the
// user before starting a +chat.
const appNoDatabaseHint = "ask the user whether to add a database through Miaoda cloud development; if confirmed, run `lark-cli apps +session-list --app-id <app_id>` and reuse an active session, or run `lark-cli apps +session-create --app-id <app_id>`; send the database requirement with `lark-cli apps +chat --app-id <app_id> --session-id <session_id> --message \"<database requirement>\"`, poll `lark-cli apps +session-get --app-id <app_id> --session-id <session_id>` until `latest_turn.status=completed`, then retry `lark-cli apps +db-table-list --app-id <app_id>`"

// withAppsHint attaches an actionable next-step hint to a typed failure,
// preserving its original classification (subtype/code/log_id). A hint already
// present on the error is kept (the upstream wording wins); only an empty hint
// is filled in. Mirrors drive.appendDriveExportRecoveryHint. err==nil and
// untyped errors pass through unchanged.
//
// Special-case appNoDatabaseCode (500002759, db command on an app with no
// database yet): rewrite the message to a user-facing explanation and force the
// cloud-development recovery hint, since the raw upstream message uses internal
// terms and any generic hint would be less actionable. This code is only
// produced by db endpoints, so the override is safe to check for every apps
// command that funnels through here.
func withAppsHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	// p points at the embedded Problem, so the mutation is reflected in err.
	if p, ok := errs.ProblemOf(err); ok {
		if p.Code == appNoDatabaseCode {
			p.Message = appNoDatabaseMessage
			p.Hint = appNoDatabaseHint
			return err
		}
		if strings.TrimSpace(p.Hint) == "" {
			p.Hint = hint
		}
		return err
	}
	return err
}

// validateRealAppID checks that --app-id is a real app ID (app_ prefix).
// meta_token values are rejected with a hint to resolve via +get first.
func validateRealAppID(appID string) error {
	if !strings.HasPrefix(appID, "app_") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			`--app-id must be an app_id starting with "app_".`,
		).WithParam("--app-id").WithHint(
			`If you have a meta_token or a /page/<token>/ link, first resolve it:
lark-cli apps +get --app-id <meta_token> -q '.data.app.app_id'
Then retry this command with the returned app_id.`,
		)
	}
	return nil
}

// rejectOutputTraversal is a defense-in-depth pre-check on a user-supplied
// --output path. The authoritative guard is the local FileIO layer
// (validate.SafeOutputPath sandboxes every write to the cwd, resolving .. and
// symlinks), so traversal is already blocked at write time; this gives an
// earlier, clearer validation error and pins the contract in the command layer.
// Empty (use server-derived default) passes through. Absolute paths and any
// ".." path component are rejected.
func rejectOutputTraversal(output string) error {
	o := strings.TrimSpace(output)
	if o == "" {
		return nil
	}
	if filepath.IsAbs(o) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--output must be a relative path within the current directory, got %q", o).WithParam("--output")
	}
	for _, seg := range strings.Split(filepath.Clean(o), string(filepath.Separator)) {
		if seg == ".." {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--output must not contain .. path traversal, got %q", o).WithParam("--output")
		}
	}
	return nil
}
