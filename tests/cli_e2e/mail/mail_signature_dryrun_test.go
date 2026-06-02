// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"strconv"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMail_SignatureWriteDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	tests := []struct {
		name       string
		args       []string
		wantMethod []string
		wantURL    []string
	}{
		{
			name: "create",
			args: []string{
				"mail", "+signature-create",
				"--name", "Agent",
				"--content", "<p>Regards</p>",
				"--dry-run",
			},
			wantMethod: []string{"POST"},
			wantURL:    []string{"/open-apis/mail/v1/user_mailboxes/me/settings/signatures"},
		},
		{
			name: "update",
			args: []string{
				"mail", "+signature-update",
				"--signature-id", "123",
				"--set-name", "Agent",
				"--dry-run",
			},
			wantMethod: []string{"GET", "PUT"},
			wantURL: []string{
				"/open-apis/mail/v1/user_mailboxes/me/settings/signatures",
				"/open-apis/mail/v1/user_mailboxes/me/settings/signatures/123",
			},
		},
		{
			name: "delete",
			args: []string{
				"mail", "+signature-delete",
				"--signature-id", "123",
				"--dry-run",
			},
			wantMethod: []string{"GET", "DELETE", "GET"},
			wantURL: []string{
				"/open-apis/mail/v1/user_mailboxes/me/settings/signatures",
				"/open-apis/mail/v1/user_mailboxes/me/settings/signatures/123",
				"/open-apis/mail/v1/user_mailboxes/me/settings/signatures",
			},
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			if gotCount := int(gjson.Get(result.Stdout, "api.#").Int()); gotCount != len(tt.wantMethod) {
				t.Fatalf("api count = %d, want %d\nstdout:\n%s", gotCount, len(tt.wantMethod), result.Stdout)
			}
			for i := range tt.wantMethod {
				idx := strconv.Itoa(i)
				if got := gjson.Get(result.Stdout, "api."+idx+".method").String(); got != tt.wantMethod[i] {
					t.Fatalf("api[%d].method = %q, want %q\nstdout:\n%s", i, got, tt.wantMethod[i], result.Stdout)
				}
				if got := gjson.Get(result.Stdout, "api."+idx+".url").String(); got != tt.wantURL[i] {
					t.Fatalf("api[%d].url = %q, want %q\nstdout:\n%s", i, got, tt.wantURL[i], result.Stdout)
				}
			}
		})
	}
}
