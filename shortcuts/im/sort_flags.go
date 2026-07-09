// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import "github.com/larksuite/cli/shortcuts/common"

// aliasFlagValue handles a renamed sort flag whose old name is kept as a silent
// alias. It returns (oldValue, true) only when the old flag was explicitly used
// and the new one was not; otherwise ("", false) — meaning "no old flag, or both
// given (new wins), so use the new-flag logic". Pure function, no IO: callable
// from DryRun, Execute, and minimal test fixtures alike. Never prints anything.
func aliasFlagValue(rt *common.RuntimeContext, oldName, newName string) (string, bool) {
	if rt.Changed(oldName) && !rt.Changed(newName) {
		return rt.Str(oldName), true
	}
	return "", false
}
