// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestMessagePullShortcutsDeclareReactionScopeAsConditional(t *testing.T) {
	shortcuts := []common.Shortcut{
		ImChatMessageList,
		ImMessagesMGet,
		ImThreadsMessagesList,
		ImMessagesSearch,
	}

	for _, shortcut := range shortcuts {
		for _, identity := range []string{"user", "bot"} {
			t.Run(shortcut.Command+"/"+identity, func(t *testing.T) {
				if slices.Contains(shortcut.ScopesForIdentity(identity), messageReactionReadScope) {
					t.Fatalf("ScopesForIdentity(%q) contains conditional reaction scope", identity)
				}
				if !slices.Contains(shortcut.ConditionalScopesForIdentity(identity), messageReactionReadScope) {
					t.Fatalf("ConditionalScopesForIdentity(%q) missing %q", identity, messageReactionReadScope)
				}
				if !slices.Contains(shortcut.DeclaredScopesForIdentity(identity), messageReactionReadScope) {
					t.Fatalf("DeclaredScopesForIdentity(%q) missing %q", identity, messageReactionReadScope)
				}
			})
		}
	}
}

func TestChatMessagesListNoReactionsSkipsReactionScopePreflight(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	f.Credential = credential.NewCredentialProvider(nil, nil, scopedTokenResolver{
		scopes: "im:message.group_msg:get_as_user im:message.p2p_msg:get_as_user",
	}, nil)

	err := mountAndRunIMShortcut(t, ImChatMessageList, f,
		"+chat-messages-list",
		"--chat-id", "oc_test",
		"--as", "user",
		"--no-reactions",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("--no-reactions should not require %s: %v", messageReactionReadScope, err)
	}
}

func TestChatMessagesListDefaultRequiresReactionScope(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	f.Credential = credential.NewCredentialProvider(nil, nil, scopedTokenResolver{
		scopes: "im:message.group_msg:get_as_user im:message.p2p_msg:get_as_user",
	}, nil)

	err := mountAndRunIMShortcut(t, ImChatMessageList, f,
		"+chat-messages-list",
		"--chat-id", "oc_test",
		"--as", "user",
		"--dry-run",
	)
	if err == nil {
		t.Fatalf("default enrichment should require %s", messageReactionReadScope)
	}
	var permErr *errs.PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("expected *errs.PermissionError, got %T", err)
	}
	if permErr.Subtype != errs.SubtypeMissingScope {
		t.Fatalf("Subtype = %q, want %q", permErr.Subtype, errs.SubtypeMissingScope)
	}
	if !strings.Contains(err.Error(), messageReactionReadScope) {
		t.Fatalf("error = %q, want missing scope %q", err, messageReactionReadScope)
	}
}

func mountAndRunIMShortcut(t *testing.T, shortcut common.Shortcut, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	parent := &cobra.Command{Use: "im"}
	shortcut.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	return parent.Execute()
}
