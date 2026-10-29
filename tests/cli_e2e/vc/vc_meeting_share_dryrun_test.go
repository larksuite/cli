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

func vcMeetingShareBinary(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "lark-cli")
	buildCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	build := exec.CommandContext(buildCtx, "go", "build", "-o", bin, ".")
	build.Dir = vcContractPath(t)
	output, err := build.CombinedOutput()
	require.NoErrorf(t, err, "build lark-cli: %s", output)
	return bin
}

func TestVCMeetingShareDryRun(t *testing.T) {
	setVCDryRunEnv(t)
	binary := vcMeetingShareBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		BinaryPath: binary,
		Args: []string{
			"vc", "+meeting-share",
			"--meeting-id", "7694644829808070175",
			"--action", "start",
			"--doc-url", "https://example.feishu.cn/docx/docx_token",
			"--as", "bot",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, "bot", gjson.Get(result.Stdout, "identity").String())
	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/vc/v1/bots/meeting_share", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	require.Equal(t, "7694644829808070175", clie2e.DryRunGet(result.Stdout, "api.0.body.meeting_id").String())
	require.Equal(t, "document", clie2e.DryRunGet(result.Stdout, "api.0.body.share_type").String())
	require.Equal(t, "https://example.feishu.cn/docx/docx_token", clie2e.DryRunGet(result.Stdout, "api.0.body.document.doc_url").String())
}

func TestVCMeetingShareStatusDryRun(t *testing.T) {
	setVCDryRunEnv(t)
	binary := vcMeetingShareBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		BinaryPath: binary,
		Args: []string{
			"vc", "+meeting-share-status",
			"--meeting-id", "7694644829808070175",
			"--as", "user",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, "user", gjson.Get(result.Stdout, "identity").String())
	require.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/vc/v1/bots/meeting_share_status", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	require.Equal(t, "7694644829808070175", clie2e.DryRunGet(result.Stdout, "api.0.params.meeting_id").String())
}
