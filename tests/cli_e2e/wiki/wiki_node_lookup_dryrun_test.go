// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"strconv"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestWikiMutationNodeLookupDryRun(t *testing.T) {
	setWikiNodeCreateDryRunEnv(t)
	for _, tt := range []struct {
		name, method, path string
		args               []string
		tokens             []string
		body               map[string]string
	}{
		{"create", "POST", "/open-apis/wiki/v2/spaces/<resolved_space_id>/nodes", []string{"+node-create", "--parent-node-token", "wik_parent"}, []string{"wik_parent"}, map[string]string{"parent_node_token": "<resolved_parent_node_token>"}},
		{"move", "POST", "/open-apis/wiki/v2/spaces/<resolved_source_space_id>/nodes/<resolved_node_token>/move", []string{"+move", "--node-token", "wik_source", "--target-parent-token", "wik_parent"}, []string{"wik_source", "wik_parent"}, map[string]string{"target_parent_token": "<resolved_parent_node_token>"}},
		{"delete wiki", "DELETE", "/open-apis/wiki/v2/spaces/<resolved_space_id>/nodes/<resolved_node_token>", []string{"+node-delete", "--node-token", "wik_source", "--obj-type", "wiki"}, []string{"wik_source"}, map[string]string{"obj_type": "wiki"}},
		{"delete document URL", "DELETE", "/open-apis/wiki/v2/spaces/<resolved_space_id>/nodes/<resolved_obj_token>", []string{"+node-delete", "--node-token", "https://example.com/docx/doc_source"}, []string{"doc_source"}, map[string]string{"obj_type": "docx"}},
		{"move explicit space", "POST", "/open-apis/wiki/v2/spaces/space_src/nodes/<resolved_node_token>/move", []string{"+move", "--node-token", "doc_source", "--source-space-id", "space_src", "--target-space-id", "space_dst"}, []string{"doc_source"}, map[string]string{"target_space_id": "space_dst"}},
		{"delete explicit space", "DELETE", "/open-apis/wiki/v2/spaces/space_src/nodes/<resolved_node_token>", []string{"+node-delete", "--node-token", "doc_source", "--obj-type", "wiki", "--space-id", "space_src"}, []string{"doc_source"}, map[string]string{"obj_type": "wiki"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			args := append([]string{"wiki"}, tt.args...)
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append(args, "--dry-run"), DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			if tt.method == "DELETE" {
				description := clie2e.DryRunGet(result.Stdout, "description").String()
				require.Contains(t, description, "always resolve the target via node_by_token")
				require.Contains(t, description, "requires Wiki node read access, including with --space-id")
			}
			for i, token := range tt.tokens {
				step := "api." + strconv.Itoa(i)
				require.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, step+".method").String())
				require.Equal(t, "/open-apis/wiki/v2/spaces/node_by_token", clie2e.DryRunGet(result.Stdout, step+".url").String())
				params := clie2e.DryRunGet(result.Stdout, step+".params").Map()
				require.Len(t, params, 1)
				require.Equal(t, token, params["token"].String())
			}
			step := "api." + strconv.Itoa(len(tt.tokens))
			require.Equal(t, tt.method, clie2e.DryRunGet(result.Stdout, step+".method").String())
			require.Equal(t, tt.path, clie2e.DryRunGet(result.Stdout, step+".url").String())
			for key, value := range tt.body {
				require.Equal(t, value, clie2e.DryRunGet(result.Stdout, step+".body."+key).String())
			}
		})
	}
}
