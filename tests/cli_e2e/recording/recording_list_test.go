// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package recording

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRecordingListDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	t.Run("standalone shortcut and date bounds", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"recording", "+list", "--start", "2026-09-01T00:00:00.123+08:00", "--end", "2026-09-02T00:00:00.456+08:00", "--page-size", "2", "--page-token", "opaque+cursor=", "--dry-run"}, DefaultAs: "user"})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		for path, want := range map[string]string{"identity": "user", "api.0.method": "GET", "api.0.url": "/open-apis/vc/v1/recordings", "api.0.params.page_size": "2", "api.0.params.page_token": "opaque+cursor=", "api.0.params.start_time_ms": "1788192000123", "api.0.params.end_time_ms": "1788278400456"} {
			require.Equal(t, want, clie2e.DryRunGet(result.Stdout, path).String(), path)
		}
	})
	t.Run("bot rejected", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"recording", "+list", "--dry-run"}, DefaultAs: "bot"})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		require.Equal(t, "--as", gjson.Get(result.Stderr, "error.param").String())
	})
	t.Run("help exposes shortcut without catalog method", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"recording", "--help"}})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "+list")
		require.NotContains(t, result.Stdout, "recordings list")
	})
}

// This read-only workflow permits an empty account. Nonempty artifacts, exact
// cursor forwarding and error paths are asserted with deterministic API mocks.
func TestRecordingListLive(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"recording", "+list", "--page-size", "1", "--page-all", "--page-limit", "2"}, DefaultAs: "user"})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.True(t, gjson.Get(result.Stdout, "data.recordings").IsArray())
	require.Equal(t, "user", gjson.Get(result.Stdout, "identity").String())
	require.LessOrEqual(t, gjson.Get(result.Stdout, "meta.pagination.pages").Int(), int64(2))
	require.GreaterOrEqual(t, gjson.Get(result.Stdout, "meta.pagination.pages").Int(), int64(1))
	for _, item := range gjson.Get(result.Stdout, "data.recordings").Array() {
		require.Equal(t, gjson.String, item.Get("recording_id").Type)
		for _, a := range item.Get("artifacts").Array() {
			require.Equal(t, gjson.Number, a.Get("type").Type)
			require.Equal(t, gjson.String, a.Get("artifact_id").Type)
		}
	}
	if gjson.Get(result.Stdout, "data.has_more").Bool() {
		require.False(t, gjson.Get(result.Stdout, "meta.pagination.complete").Bool())
		token := gjson.Get(result.Stdout, "data.page_token").String()
		require.NotEmpty(t, token)
		next, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"recording", "+list", "--page-size", "1", "--page-token", token}, DefaultAs: "user"})
		require.NoError(t, err)
		next.AssertExitCode(t, 0)
		next.AssertStdoutStatus(t, true)
		require.True(t, gjson.Get(next.Stdout, "data.recordings").IsArray())
	}
}
