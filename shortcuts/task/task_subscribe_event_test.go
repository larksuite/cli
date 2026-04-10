// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestSubscribeTaskEvent(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		wantParts []string
	}{
		{
			name:      "execute json",
			mode:      "execute",
			wantParts: []string{`"ok": true`},
		},
		{
			name:      "dry run",
			mode:      "dryrun",
			wantParts: []string{"POST /open-apis/task/v2/task_v2/task_subscription"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.mode {
			case "execute":
				f, stdout, _, reg := taskShortcutTestFactory(t)
				warmTenantToken(t, f, reg)
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/task_v2/task_subscription",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{},
					},
				})

				s := SubscribeTaskEvent
				s.AuthTypes = []string{"bot", "user"}
				err := runMountedTaskShortcut(t, s, []string{"+subscribe-event", "--as", "bot", "--format", "json"}, f, stdout)
				if err != nil {
					t.Fatalf("runMountedTaskShortcut() error = %v", err)
				}

				out := stdout.String()
				outNorm := strings.ReplaceAll(out, `":"`, `": "`)
				for _, want := range tt.wantParts {
					if !strings.Contains(out, want) && !strings.Contains(outNorm, want) {
						t.Fatalf("output missing %q: %s", want, out)
					}
				}
			case "dryrun":
				runtime := common.TestNewRuntimeContextWithIdentity(&cobra.Command{Use: "test"}, taskTestConfig(t), "user")
				out := SubscribeTaskEvent.DryRun(nil, runtime).Format()
				for _, want := range tt.wantParts {
					if !strings.Contains(out, want) {
						t.Fatalf("dry run output missing %q: %s", want, out)
					}
				}
			}
		})
	}
}
