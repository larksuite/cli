// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package shortcuts

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// Exercise the shipped support table through real command registration: Agent
// Employee access must not loosen the ordinary tenant/bot identity restriction.
func TestHTMLShortcutPrincipalAccess(t *testing.T) {
	for _, command := range []string{"+publish", "+fetch"} {
		for _, principal := range []struct {
			name string
			kind core.CredentialKind
			as   string
		}{
			{"AgentEmployee", core.CredentialAAT, "user"},
			{"User", core.CredentialUAT, "user"},
			{"Bot", core.CredentialTAT, "bot"},
		} {
			t.Run(command+"/"+principal.name, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				cmdutil.TestChdir(t, t.TempDir())
				if err := os.WriteFile("index.html", []byte("<html>hello</html>"), 0600); err != nil {
					t.Fatal(err)
				}
				f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
					AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
					CredentialKind: principal.kind, PrincipalOpenID: "ou_test", UserOpenId: "ou_test",
				})
				program := &cobra.Command{Use: "root", SilenceErrors: true, SilenceUsage: true}
				RegisterShortcutsWithContext(context.Background(), program, f)
				args := []string{"html", command, "--as", principal.as, "--dry-run"}
				if command == "+publish" {
					args = append(args, "--file-path", "index.html")
				} else {
					args = append(args, "--token", "html_test", "--output", "source.zip")
				}
				program.SetArgs(args)
				err := program.ExecuteContext(context.Background())
				if principal.kind == core.CredentialTAT {
					var validation *errs.ValidationError
					if !errors.As(err, &validation) || !strings.Contains(validation.Message, "only supports: user") {
						t.Fatalf("want bot identity rejection, got %v", err)
					}
					if stdout.Len() != 0 {
						t.Fatalf("rejected bot produced success output: %s", stdout.String())
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var preview struct {
					OK     bool `json:"ok"`
					DryRun bool `json:"dry_run"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil || !preview.OK || !preview.DryRun {
					t.Fatalf("want successful preview, got %s (decode error: %v)", stdout.String(), err)
				}
			})
		}
	}
}
