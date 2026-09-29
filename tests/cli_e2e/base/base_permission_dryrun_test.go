// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestBasePermissionShortcutsDryRun(t *testing.T) {
	for _, tc := range []struct {
		command, method, suffix string
		args                    []string
		key, value              string
	}{
		{"+role-member-list", "GET", "/roles/rol_test/members", []string{"--role-id", "rol_test"}, "params.page_size", "50"},
		{"+role-member-list", "GET", "/roles/rol_test/members", []string{"--role-id", "rol_test", "--page-size", "7", "--page-token", "14"}, "params.page_token", "14"},
		{"+role-member-add", "POST", "/roles/rol_test/members/batch_create", []string{"--role-id", "rol_test", "--user-id", "ou_test", "--department-id", "od_test", "--chat-id", "oc_test"}, "body.members.2.member_type", "openchat"},
		{"+role-member-remove", "POST", "/roles/rol_test/members/batch_delete", []string{"--role-id", "rol_test", "--chat-id", "oc_test"}, "body.members.0.member_id", "oc_test"},
		{"+advperm-share-get", "GET", "/advperm/share", nil, "method", "GET"},
		{"+advperm-share-update", "PUT", "/advperm/share", []string{"--enable-share=false"}, "body.enable_share", "false"},
		{"+advperm-apply", "POST", "/advperm/apply", []string{"--message", "Please review"}, "body.message", "Please review"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			args := append([]string{"base", tc.command, "--base-token", "base_test"}, tc.args...)
			result := runBaseDryRun(t, 0, args...)
			require.Equal(t, tc.method, clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/base/v3/bases/base_test"+tc.suffix, clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.Equal(t, tc.value, clie2e.DryRunGet(result.Stdout, "api.0."+tc.key).String())
		})
	}
}
