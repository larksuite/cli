// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

func requireDriveBotCurrentUserForCreate(runtime *common.RuntimeContext, command string) error {
	if runtime == nil || !runtime.IsBot() || strings.TrimSpace(runtime.UserOpenId()) != "" {
		return nil
	}
	return errs.NewValidationError(
		errs.SubtypeFailedPrecondition,
		"%s in bot mode creates a new Drive resource and must know the current CLI user's open_id before upload, otherwise the created resource cannot be auto-granted to that user",
		command,
	).WithHint(
		"run `lark-cli auth login` first, or rerun with `--as user`; this prevents creating a bot-owned resource that the user cannot edit",
	)
}
