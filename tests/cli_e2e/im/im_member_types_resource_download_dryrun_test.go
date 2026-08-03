// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIMChatMembersListMemberTypesCompatibilityDryRun(t *testing.T) {
	setFlagAliasDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	run := func(t *testing.T, value string) *clie2e.Result {
		t.Helper()
		args := []string{"im", "+chat-members-list", "--chat-id", "oc_dryrun"}
		if value != "" {
			args = append(args, "--member-types", value)
		}
		args = append(args, "--dry-run")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		return result
	}

	omitted := run(t, "")
	for _, value := range []string{"all", "ALL", "all,user"} {
		t.Run(value, func(t *testing.T) {
			result := run(t, value)
			require.JSONEq(t, omitted.Stdout, result.Stdout)
			require.Contains(t, result.Stderr, "means no filter (same as omitting the flag)")
			require.NotContains(t, result.Stdout, "means no filter")
		})
	}

	for _, tc := range []struct {
		compat    string
		canonical string
		wantNote  string
	}{
		{compat: "users", canonical: "user", wantNote: `note: --member-types "users" is accepted as "user"`},
		{compat: "bots", canonical: "bot", wantNote: `note: --member-types "bots" is accepted as "bot"`},
		{compat: "Users", canonical: "user", wantNote: `note: --member-types "Users" is accepted as "user"`},
	} {
		t.Run(tc.compat, func(t *testing.T) {
			result := run(t, tc.compat)
			canonical := run(t, tc.canonical)
			require.JSONEq(t, canonical.Stdout, result.Stdout)
			require.Equal(t, 1, strings.Count(result.Stderr, tc.wantNote))
			require.NotContains(t, result.Stdout, "is accepted as")
		})
	}

	invalid, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"im", "+chat-members-list", "--chat-id", "oc_dryrun", "--member-types", "xxx", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	invalid.AssertExitCode(t, 2)
	require.Empty(t, invalid.Stdout)
	require.Equal(t, `invalid --member-types value "xxx": expected one of user, bot, all`, gjson.Get(invalid.Stderr, "error.message").String())
	require.Equal(t, "validation", gjson.Get(invalid.Stderr, "error.type").String())
	require.Equal(t, "invalid_argument", gjson.Get(invalid.Stderr, "error.subtype").String())
	require.Equal(t, "--member-types", gjson.Get(invalid.Stderr, "error.param").String())
}

func TestIMMessagesResourcesDownloadRequiredFlagsDryRun(t *testing.T) {
	setFlagAliasDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	missing, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"im", "+messages-resources-download", "--message-id", "om_dryrun", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	missing.AssertExitCode(t, 2)
	require.Empty(t, missing.Stdout)
	require.Equal(t, "--file-key and --type are required", gjson.Get(missing.Stderr, "error.message").String())
	hint := gjson.Get(missing.Stderr, "error.hint").String()
	require.Contains(t, hint, "+messages-mget")
	require.Contains(t, hint, "--download-resources")
	require.Equal(t, int64(2), gjson.Get(missing.Stderr, "error.params.#").Int())
	require.Equal(t, "validation", gjson.Get(missing.Stderr, "error.type").String())
	require.Equal(t, "invalid_argument", gjson.Get(missing.Stderr, "error.subtype").String())
	require.Equal(t, "--file-key", gjson.Get(missing.Stderr, "error.params.0.name").String())
	require.Equal(t, "--type", gjson.Get(missing.Stderr, "error.params.1.name").String())

	complete, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+messages-resources-download", "--message-id", "om_dryrun",
			"--file-key", "img_dryrun", "--type", "image", "--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	complete.AssertExitCode(t, 0)
	require.Equal(t, "GET", clie2e.DryRunGet(complete.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/im/v1/messages/om_dryrun/resources/img_dryrun", clie2e.DryRunGet(complete.Stdout, "api.0.url").String())
	require.Equal(t, "image", clie2e.DryRunGet(complete.Stdout, "api.0.params.type").String())
}

func TestIMMessagesResourcesDownloadHelpMarksManualRequiredFlags(t *testing.T) {
	setFlagAliasDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"im", "+messages-resources-download", "--help"}})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.True(t, regexp.MustCompile(`(?m)^\s+--file-key\s+string\s+.*required`).MatchString(result.Stdout), "--file-key help does not mark it required:\n%s", result.Stdout)
	require.True(t, regexp.MustCompile(`(?m)^\s+--type\s+string\s+.*required`).MatchString(result.Stdout), "--type help does not mark it required:\n%s", result.Stdout)
}
