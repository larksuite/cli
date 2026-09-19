// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// TestUserMailboxShortcutsRequireUserIdentity verifies that shortcuts backed
// exclusively by user_mailbox.* APIs reject bot identity before making any
// API call. The error must contain "not supported" so the caller knows the
// issue is identity, not a bad parameter.
func TestUserMailboxShortcutsRequireUserIdentity(t *testing.T) {
	tests := []struct {
		shortcut common.Shortcut
		args     []string // minimum args to pass cobra's required-flag check
	}{
		{
			shortcut: MailTriage,
			args:     []string{"+triage", "--as", "bot"},
		},
		{
			shortcut: MailMessages,
			args:     []string{"+messages", "--as", "bot", "--message-ids", "dummy"},
		},
		{
			shortcut: MailTemplateCreate,
			args:     []string{"+template-create", "--as", "bot", "--name", "dummy"},
		},
		{
			shortcut: MailThread,
			args:     []string{"+thread", "--as", "bot", "--thread-id", "dummy"},
		},
		{
			shortcut: MailTemplateUpdate,
			args:     []string{"+template-update", "--as", "bot"},
		},
		{
			shortcut: MailMessage,
			args:     []string{"+message", "--as", "bot", "--message-id", "dummy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.shortcut.Command, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
				AppID:     "test-app",
				AppSecret: "test-secret",
				Brand:     core.BrandFeishu,
			})

			parent := &cobra.Command{Use: "mail"}
			tt.shortcut.Mount(parent, f)
			parent.SetArgs(tt.args)
			parent.SilenceErrors = true
			parent.SilenceUsage = true

			err := parent.Execute()
			if err == nil {
				t.Fatal("expected error for bot identity, got nil")
			}
			if !strings.Contains(err.Error(), "not supported") {
				t.Errorf("expected 'not supported' in error, got: %v", err)
			}
		})
	}
}
