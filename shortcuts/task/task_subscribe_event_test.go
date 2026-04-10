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
		args      []string
		register  func(*httpmock.Registry)
		wantErr   bool
		wantParts []string
	}{
		{
			name: "execute json (user identity)",
			mode: "execute",
			args: []string{"+subscribe-event", "--as", "user", "--format", "json"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/task_v2/task_subscription",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{},
					},
				})
			},
			wantParts: []string{`"ok": true`},
		},
		{
			name: "execute json (bot identity)",
			mode: "execute",
			args: []string{"+subscribe-event", "--as", "bot", "--format", "json"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/task_v2/task_subscription",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{},
					},
				})
			},
			wantParts: []string{`"ok": true`},
		},
		{
			name: "execute api error",
			mode: "execute",
			args: []string{"+subscribe-event", "--as", "bot", "--format", "json"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/task_v2/task_subscription",
					Body: map[string]interface{}{
						"code": 401,
						"msg":  "Unauthorized",
						"error": map[string]interface{}{
							"log_id": "test-log-id",
						},
					},
				})
			},
			wantErr:   true,
			wantParts: []string{"Unauthorized"},
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
				if tt.register != nil {
					tt.register(reg)
				}

				err := runMountedTaskShortcut(t, SubscribeTaskEvent, tt.args, f, stdout)
				if tt.wantErr {
					if err == nil {
						t.Fatal("expected error, got nil")
					}
					out := err.Error()
					for _, want := range tt.wantParts {
						if !strings.Contains(out, want) {
							t.Fatalf("error missing %q: %s", want, out)
						}
					}
					return
				}
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
