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

func TestIMChatSearchTypesGroupDryRunMatchesChatModesGroup(t *testing.T) {
	setFlagAliasDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	typesResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"im", "+chat-search", "--query", "team", "--types", "group", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	typesResult.AssertExitCode(t, 0)

	canonicalResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"im", "+chat-search", "--query", "team", "--chat-modes", "group", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	canonicalResult.AssertExitCode(t, 0)

	require.JSONEq(t, canonicalResult.Stdout, typesResult.Stdout)
	require.Equal(t, "default", clie2e.DryRunGet(typesResult.Stdout, "api.0.body.filter.chat_modes.0").String())
	require.Equal(t, 1, strings.Count(typesResult.Stderr, "note: --types on +chat-search maps to --chat-modes"))
	require.NotContains(t, typesResult.Stdout, "maps to --chat-modes")
}

func TestIMChatSearchCanonicalChatModesWinsOverTypes(t *testing.T) {
	setFlagAliasDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"im", "+chat-search", "--query", "team", "--types", "p2p", "--chat-modes", "topic", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, "thread", clie2e.DryRunGet(result.Stdout, "api.0.body.filter.chat_modes.0").String())
	require.NotContains(t, result.Stderr, "--types on +chat-search maps")
}

func TestIMChatSearchTypesValidationErrors(t *testing.T) {
	setFlagAliasDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	for _, typesValue := range []string{"p2p", "group,p2p"} {
		t.Run(typesValue, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      []string{"im", "+chat-search", "--query", "team", "--types", typesValue, "--dry-run"},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			require.Empty(t, result.Stdout)
			message := gjson.Get(result.Stderr, "error.message").String()
			require.Contains(t, message, "service does not support p2p")
			require.Contains(t, message, "im +chat-list --types p2p")
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
			require.Equal(t, "--types", gjson.Get(result.Stderr, "error.param").String())
		})
	}

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"im", "+chat-search", "--query", "team", "--types", "xxx", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Empty(t, result.Stdout)
	message := gjson.Get(result.Stderr, "error.message").String()
	require.Contains(t, message, "--chat-modes (group|topic)")
	require.Contains(t, message, "--search-types (private|external|public_joined|public_not_joined)")
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
	require.Equal(t, "--types", gjson.Get(result.Stderr, "error.param").String())
}

func TestIMChatSearchTypesHiddenFromHelp(t *testing.T) {
	setFlagAliasDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"im", "+chat-search", "--help"}})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.False(t, regexp.MustCompile(`(?m)^\s+--types(?:\s|$)`).MatchString(result.Stdout), "--types leaked into help:\n%s", result.Stdout)
}
