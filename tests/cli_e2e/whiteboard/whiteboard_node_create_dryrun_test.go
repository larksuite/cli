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

func TestWhiteboardNodeCreateDryRun_RequestShape(t *testing.T) {
	setWhiteboardDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"whiteboard", "+node-create",
			"--whiteboard-token", "wbcnCreateDryRun",
			"--source", `{"nodes":[{"id":"tmpNode","type":"composite_shape","x":0,"y":0,"width":260,"height":45,"text":{"text":"hello","font_weight":"regular","font_size":14,"horizontal_align":"center","vertical_align":"mid"},"style":{"border_color":"#3370ff","border_width":"narrow","border_style":"solid","fill_color":"#e8f3ff"},"composite_shape":{"type":"round_rect"}}]}`,
			"--idempotent-token", "create-token-12345",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, int64(1), clie2e.DryRunGet(out, "api.#").Int(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	gotURL := clie2e.DryRunGet(out, "api.0.url").String()
	if !strings.HasPrefix(gotURL, "/open-apis/board/v1/whiteboards/") || !strings.HasSuffix(gotURL, "/nodes") || strings.Contains(gotURL, "wbcnCreateDryRun") {
		t.Fatalf("url=%q, want masked board whiteboard nodes URL\nstdout:\n%s", gotURL, out)
	}
	require.Equal(t, "create-token-12345", clie2e.DryRunGet(out, "api.0.params.client_token").String(), out)
	require.Equal(t, "composite_shape", clie2e.DryRunGet(out, "api.0.body.nodes.0.type").String(), out)
	require.Equal(t, "round_rect", clie2e.DryRunGet(out, "api.0.body.nodes.0.composite_shape.type").String(), out)
}
