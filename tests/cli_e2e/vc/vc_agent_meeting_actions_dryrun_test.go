// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestVCAgentMeetingActionsDryRun(t *testing.T) {
	setVCDryRunEnv(t)

	repoRoot := vcContractPath(t)
	bin := filepath.Join(t.TempDir(), "lark-cli")
	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", bin, ".")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build lark-cli: %v\n%s", err, output)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name      string
		identity  string
		args      []string
		path      string
		bodyPath  string
		bodyValue string
	}{
		{
			name:      "join with lobby",
			identity:  "user",
			args:      []string{"vc", "+meeting-join", "--meeting-number", "123456789", "--enable-lobby", "--dry-run"},
			path:      "/open-apis/vc/v1/bots/join",
			bodyPath:  "enable_lobby",
			bodyValue: "true",
		},
		{
			name:     "join without lobby",
			identity: "user",
			args:     []string{"vc", "+meeting-join", "--meeting-number", "123456789", "--dry-run"},
			path:     "/open-apis/vc/v1/bots/join",
			bodyPath: "enable_lobby",
		},
		{
			name:     "start with lobby disabled",
			identity: "bot",
			args:     []string{"vc", "+meeting-join", "--meeting-number", "123456789", "--action", "start", "--enable-lobby=false", "--dry-run"},
			path:     "/open-apis/vc/v1/bots/join",
			bodyPath: "enable_lobby",
		},
		{
			name:      "start and join",
			identity:  "bot",
			args:      []string{"vc", "+meeting-join", "--meeting-number", "123456789", "--action", "start", "--dry-run"},
			path:      "/open-apis/vc/v1/bots/join",
			bodyPath:  "action",
			bodyValue: "2",
		},
		{
			name:      "invite all suggested",
			identity:  "bot",
			args:      []string{"vc", "+meeting-invite", "--meeting-id", "7628568141510692381", "--type", "ALL_SUGGESTED", "--dry-run"},
			path:      "/open-apis/vc/v1/bots/invite",
			bodyPath:  "meeting_id",
			bodyValue: "7628568141510692381",
		},
		{
			name:      "end meeting",
			identity:  "bot",
			args:      []string{"vc", "+meeting-end", "--meeting-id", "7628568141510692381", "--dry-run"},
			path:      "/open-apis/vc/v1/bots/end",
			bodyPath:  "meeting_id",
			bodyValue: "7628568141510692381",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				BinaryPath: bin,
				Args:       testCase.args,
				DefaultAs:  testCase.identity,
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Equal(t, testCase.identity, clie2e.DryRunGet(result.Stdout, "identity").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, int64(1), clie2e.DryRunGet(result.Stdout, "api.#").Int(), "stdout:\n%s", result.Stdout)
			require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, testCase.path, clie2e.DryRunGet(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, testCase.bodyValue, clie2e.DryRunGet(result.Stdout, "api.0.body."+testCase.bodyPath).String(), "stdout:\n%s", result.Stdout)
			if testCase.bodyValue == "" {
				require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body."+testCase.bodyPath).Exists(), "stdout:\n%s", result.Stdout)
			}
		})
	}
}

func TestVCMeetingJoinDryRunRejectsBotLobby(t *testing.T) {
	setVCDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"vc", "+meeting-join", "--meeting-number", "123456789", "--enable-lobby", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, "--enable-lobby", gjson.Get(result.Stderr, "error.param").String(), "stderr:\n%s", result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "not supported for bot identity", "stderr:\n%s", result.Stderr)
	require.Empty(t, result.Stdout)
}
