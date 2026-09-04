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

func TestWhiteboardNodeDeleteDryRun_RequestShape(t *testing.T) {
	setWhiteboardDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"whiteboard", "+node-delete",
			"--whiteboard-token", "wbcnDeleteDryRun",
			"--node-ids", "nodeA,nodeB",
			"--idempotent-token", "delete-token-12345",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, int64(1), clie2e.DryRunGet(out, "api.#").Int(), out)
	require.Equal(t, "DELETE", clie2e.DryRunGet(out, "api.0.method").String(), out)
	gotURL := clie2e.DryRunGet(out, "api.0.url").String()
	if !strings.HasPrefix(gotURL, "/open-apis/board/v1/whiteboards/") ||
		!strings.HasSuffix(gotURL, "/nodes/batch_delete") ||
		strings.Contains(gotURL, "wbcnDeleteDryRun") {
		t.Fatalf("url=%q, want masked board whiteboard batch delete URL\nstdout:\n%s", gotURL, out)
	}
	require.Equal(t, "delete-token-12345", clie2e.DryRunGet(out, "api.0.params.client_token").String(), out)
	require.Equal(t, "nodeA", clie2e.DryRunGet(out, "api.0.body.ids.0").String(), out)
	require.Equal(t, "nodeB", clie2e.DryRunGet(out, "api.0.body.ids.1").String(), out)
}
