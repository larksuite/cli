// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseFieldGroupsDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	const path = "/open-apis/base/v3/bases/base_test/tables/tbl_test/field_groups"
	const batch = `{"operations":[{"operation":"add","name":"Contact","children":[{"type":"field","id":"Email"}]},{"operation":"rename","id":"Contact","name":"Contacts"},{"operation":"set_description","id":"Contacts","description":"**Contact details**"},{"operation":"add_children","id":"Contacts","children":[{"type":"field","name":"Phone"}]},{"operation":"remove_children","id":"Contacts","children":[{"type":"field","id":"Email"}]},{"operation":"delete","id":"Contacts"}]}`

	for _, source := range []string{"list", "inline", "file", "stdin"} {
		t.Run(source, func(t *testing.T) {
			command := "+field-group-manage"
			if source == "list" {
				command = "+field-group-list"
			}
			req := clie2e.Request{
				Args:      []string{"base", command, "--base-token", "base_test", "--table-id", "tbl_test", "--dry-run"},
				DefaultAs: "user",
			}
			switch source {
			case "inline":
				req.Args = append(req.Args, "--json", batch)
			case "file":
				req.WorkDir = t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(req.WorkDir, "groups.json"), []byte(batch), 0o600))
				req.Args = append(req.Args, "--json", "@groups.json")
			case "stdin":
				req.Args = append(req.Args, "--json", "-")
				req.Stdin = []byte(batch)
			}
			result, err := clie2e.RunCmd(ctx, req)
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			result.AssertStdoutStatus(t, true)
			require.Empty(t, result.Stderr)
			require.Len(t, clie2e.DryRunGet(result.Stdout, "api").Array(), 1)
			require.Equal(t, path, clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			method := "POST"
			if source == "list" {
				method = "GET"
				require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body").Exists())
			} else {
				require.JSONEq(t, batch, clie2e.DryRunGet(result.Stdout, "api.0.body").Raw)
			}
			require.Equal(t, method, clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		})
	}
}

func TestBaseFieldGroupsRejectInvalidJSONBeforeRequest(t *testing.T) {
	for _, body := range []string{`{"operations":[`, `null`, `[]`} {
		t.Run(body, func(t *testing.T) {
			result := runBaseDryRun(t, 2, "base", "+field-group-manage",
				"--base-token", "base_test", "--table-id", "tbl_test", "--json", body)
			require.Empty(t, result.Stdout)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
			require.Equal(t, "--json", gjson.Get(result.Stderr, "error.param").String())
		})
	}
}
