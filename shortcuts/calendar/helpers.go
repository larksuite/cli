// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

const (
	PrimaryCalendarIDStr = "primary"
)

// resolveStartEnd returns (startInput, endInput) from flags with defaults.
// --start defaults to today's date, --end defaults to start date (will be resolved to end-of-day by caller).
func resolveStartEnd(runtime *common.RuntimeContext) (string, string) {
	startInput := runtime.Str("start")
	if startInput == "" {
		startInput = time.Now().Format("2006-01-02")
	}
	endInput := runtime.Str("end")
	if endInput == "" {
		endInput = startInput
	}
	return startInput, endInput
}

func hasExplicitBotFlag(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flag("as")
	return flag != nil && flag.Changed && flag.Value != nil && strings.TrimSpace(flag.Value.String()) == "bot"
}

func rejectCalendarAutoBotFallback(runtime *common.RuntimeContext) error {
	if runtime == nil || !runtime.IsBot() || hasExplicitBotFlag(runtime.Cmd) {
		return nil
	}
	if runtime.Factory == nil || !runtime.Factory.IdentityAutoDetected {
		return nil
	}

	msg := "calendar commands require a valid user login by default; when no valid user login state is available, auto identity falls back to bot and may operate on the bot calendar instead of your own. Run `lark-cli auth login --domain calendar` for your calendar, or rerun with `--as bot` if bot identity is intentional."
	hint := "restore user login: `lark-cli auth login --domain calendar`\nintentional bot usage: rerun with `--as bot`"
	return output.ErrWithHint(output.ExitAuth, "calendar_user_login_required", msg, hint)
}
