// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestWhiteboardNodeUpdateDryRun_RequestShape(t *testing.T) {
	setWhiteboardDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"whiteboard", "+node-update",
			"--whiteboard-token", "wbcnUpdateDryRun",
			"--source", `{"code":0,"msg":"success","data":{"nodes":[{"id":"nodeA","type":"text_shape","text":{"text":"hello A","content":"drop me"},"extra":true},{"id":"nodeB","type":"text_shape","text":{"text":"hello B"}}]}}`,
			"--idempotent-token", "update-token-12345",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, int64(1), clie2e.DryRunGet(out, "api.#").Int(), out)
	require.Equal(t, "PUT", clie2e.DryRunGet(out, "api.0.method").String(), out)
	gotURL := clie2e.DryRunGet(out, "api.0.url").String()
	if !strings.HasPrefix(gotURL, "/open-apis/board/v1/whiteboards/") ||
		!strings.HasSuffix(gotURL, "/nodes/batch_update") ||
		strings.Contains(gotURL, "wbcnUpdateDryRun") {
		t.Fatalf("url=%q, want masked board whiteboard batch update URL\nstdout:\n%s", gotURL, out)
	}
	require.Equal(t, "update-token-12345", clie2e.DryRunGet(out, "api.0.params.client_token").String(), out)
	require.Equal(t, "nodeA", clie2e.DryRunGet(out, "api.0.body.nodes.0.id").String(), out)
	require.Equal(t, "hello A", clie2e.DryRunGet(out, "api.0.body.nodes.0.text.text").String(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.nodes.0.text.content").Exists(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.nodes.0.extra").Exists(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.code").Exists(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.data").Exists(), out)
	require.Equal(t, "nodeB", clie2e.DryRunGet(out, "api.0.body.nodes.1.id").String(), out)
	require.Equal(t, "hello B", clie2e.DryRunGet(out, "api.0.body.nodes.1.text.text").String(), out)
}
