// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"

	"github.com/larksuite/cli/errs"
)

// appsService 是 CLI 命令的 service 前缀（lark-cli apps ...）。
const appsService = "apps"

// apiBasePath is the registered OAPI prefix for the Miaoda apps domain.
const apiBasePath = "/open-apis/spark/v1"

// appIDListHint is the shared recovery hint for commands whose most likely
// failure cause is a wrong/inaccessible --app-id. It points at +list to find
// the correct Miaoda app id. The app_/cli_ format rule is taught in
// lark-apps SKILL.md ("app_id 获取"); the hint stays lean and does not repeat it.
const appIDListHint = "verify --app-id is correct and you have access to the app; list your apps with `lark-cli apps +list`"

// withAppsHint attaches an actionable next-step hint to a typed failure,
// preserving its original classification (subtype/code/log_id). A hint already
// present on the error is kept (the upstream wording wins); only an empty hint
// is filled in. Mirrors drive.appendDriveExportRecoveryHint. err==nil and
// untyped errors pass through unchanged.
func withAppsHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	// p points at the embedded Problem, so the mutation is reflected in err.
	if p, ok := errs.ProblemOf(err); ok {
		if strings.TrimSpace(p.Hint) == "" {
			p.Hint = hint
		}
		return err
	}
	return err
}
