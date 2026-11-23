// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"net/url"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestBaseButtonRuleDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name          string
		command       string
		fieldRef      string
		wantMethod    string
		wantBody      string
		wantBodyField bool
	}{
		{
			name: "bind by ID", command: "+button-rule-bind", fieldRef: "fld_x",
			wantMethod: "PUT", wantBody: "wkf_x", wantBodyField: true,
		},
		{
			name: "bind by name", command: "+button-rule-bind", fieldRef: "按钮",
			wantMethod: "PUT", wantBody: "wkf_x", wantBodyField: true,
		},
		{
			name: "get by ID", command: "+button-rule-get", fieldRef: "fld_x", wantMethod: "GET",
		},
		{
			name: "get by name", command: "+button-rule-get", fieldRef: "按钮", wantMethod: "GET",
		},
		{
			name: "unbind by ID", command: "+button-rule-unbind", fieldRef: "fld_x",
			wantMethod: "PUT", wantBody: "", wantBodyField: true,
		},
		{
			name: "unbind by name", command: "+button-rule-unbind", fieldRef: "按钮",
			wantMethod: "PUT", wantBody: "", wantBodyField: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{
				"base", tt.command,
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--field-id", tt.fieldRef,
			}
			if tt.command == "+button-rule-bind" {
				args = append(args, "--workflow-id", "wkf_x")
			}
			args = append(args, "--dry-run")

			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/"+url.PathEscape(tt.fieldRef), clie2e.DryRunGet(out, "api.0.url").String(), out)
			require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), out)
			require.Equal(t, "Resolve --field-id as a field ID or name", clie2e.DryRunGet(out, "api.0.desc").String(), out)
			require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/%3Cresolved_field_id%3E/button_rule", clie2e.DryRunGet(out, "api.1.url").String(), out)
			require.Equal(t, tt.wantMethod, clie2e.DryRunGet(out, "api.1.method").String(), out)
			require.Equal(t, "Use the canonical field ID returned by step 1", clie2e.DryRunGet(out, "api.1.desc").String(), out)
			require.Equal(t, tt.fieldRef, clie2e.DryRunGet(out, "field_ref").String(), out)
			require.Equal(t, "<resolved_field_id>", clie2e.DryRunGet(out, "resolved_field_id").String(), out)
			if tt.wantBodyField {
				require.Equal(t, tt.wantBody, clie2e.DryRunGet(out, "api.1.body.workflow_id").String(), out)
			}
		})
	}
}
