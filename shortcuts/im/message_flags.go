// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/spf13/cobra"
)

const (
	markdownMsgTypeMessage = "markdown is an input format, not a msg_type"
	markdownMsgTypeHint    = "Replace `--msg-type markdown --content <text>` with `--markdown <text>`."
)

// installMessageFlagBehavior composes the two IM-local flag extensions used
// by send/reply. Keeping this at PostMount preserves the common shortcut flag
// model and leaves other domains' enum behavior unchanged.
func installMessageFlagBehavior(cmd *cobra.Command) {
	installMentionFlagParser(cmd)
	chainMarkdownMsgTypeRecovery(cmd)
}

// chainMarkdownMsgTypeRecovery recognizes the common category error before
// the generic enum validator. It gives the caller a mechanical rewrite while
// retaining the real OpenAPI msg_type enum for help and completion.
func chainMarkdownMsgTypeRecovery(cmd *cobra.Command) {
	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		if !c.Flags().Changed("msg-type") {
			return nil
		}
		msgType, err := c.Flags().GetString("msg-type")
		if err != nil {
			return errs.NewInternalError(
				errs.SubtypeUnknown,
				"read --msg-type: %v",
				err,
			).WithCause(err)
		}
		if !strings.EqualFold(strings.TrimSpace(msgType), "markdown") {
			return nil
		}
		return errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			markdownMsgTypeMessage,
		).WithParam("--msg-type").WithHint("%s", markdownMsgTypeHint)
	}
}
