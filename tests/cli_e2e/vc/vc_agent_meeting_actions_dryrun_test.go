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
		args      []string
		path      string
		bodyPath  string
		bodyValue string
	}{
		{
			name:      "start and join",
			args:      []string{"vc", "+meeting-join", "--meeting-number", "123456789", "--action", "start", "--dry-run"},
			path:      "/open-apis/vc/v1/bots/join",
			bodyPath:  "action",
			bodyValue: "2",
		},
		{
			name:      "invite all suggested",
			args:      []string{"vc", "+meeting-invite", "--meeting-id", "7628568141510692381", "--type", "ALL_SUGGESTED", "--dry-run"},
			path:      "/open-apis/vc/v1/bots/invite",
			bodyPath:  "meeting_id",
			bodyValue: "7628568141510692381",
		},
		{
			name:      "end meeting",
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
				DefaultAs:  "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Equal(t, int64(1), clie2e.DryRunGet(result.Stdout, "api.#").Int(), "stdout:\n%s", result.Stdout)
			require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, testCase.path, clie2e.DryRunGet(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, testCase.bodyValue, clie2e.DryRunGet(result.Stdout, "api.0.body."+testCase.bodyPath).String(), "stdout:\n%s", result.Stdout)
		})
	}
}
