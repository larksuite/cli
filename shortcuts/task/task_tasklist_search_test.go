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

func TestBuildTasklistSearchBody(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*cobra.Command)
		wantErr bool
		check   func(*testing.T, map[string]interface{})
	}{
		{
			name: "creator create-time and page token",
			setup: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("creator", "ou_creator")
				_ = cmd.Flags().Set("create-time", "-7d,+0d")
				_ = cmd.Flags().Set("page-token", "pt_tl")
			},
			check: func(t *testing.T, body map[string]interface{}) {
				filter := body["filter"].(map[string]interface{})
				createTime := filter["create_time"].(map[string]interface{})
				if body["page_token"] != "pt_tl" {
					t.Fatalf("unexpected body: %#v", body)
				}
				if filter["user_id"].([]string)[0] != "ou_creator" {
					t.Fatalf("unexpected filter: %#v", filter)
				}
				startTime, _ := createTime["start_time"].(string)
				endTime, _ := createTime["end_time"].(string)
				if startTime == "" || endTime == "" || !strings.Contains(startTime, "T") || !strings.Contains(endTime, "T") {
					t.Fatalf("unexpected create_time: %#v", createTime)
				}
			},
		},
		{
			name:    "requires query or filter",
			setup:   func(cmd *cobra.Command) {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("query", "", "")
			cmd.Flags().String("creator", "", "")
			cmd.Flags().String("create-time", "", "")
			cmd.Flags().String("page-token", "", "")
			tt.setup(cmd)

			runtime := common.TestNewRuntimeContextWithIdentity(cmd, taskTestConfig(t), "user")
			body, err := buildTasklistSearchBody(runtime)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildTasklistSearchBody() error = %v", err)
			}
			tt.check(t, body)
		})
	}
}

func TestSearchTasklist_DryRun(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*cobra.Command)
		wantParts []string
	}{
		{
			name: "valid dry run",
			setup: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("query", "Q2")
				_ = cmd.Flags().Set("page-token", "pt_tl")
			},
			wantParts: []string{"POST /open-apis/task/v2/tasklists/search", `"query":"Q2"`},
		},
		{
			name: "dry run error on invalid create time",
			setup: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("create-time", "bad-time")
			},
			wantParts: []string{"error:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("query", "", "")
			cmd.Flags().String("creator", "", "")
			cmd.Flags().String("create-time", "", "")
			cmd.Flags().String("page-token", "", "")
			tt.setup(cmd)

			runtime := common.TestNewRuntimeContextWithIdentity(cmd, taskTestConfig(t), "user")
			if !strings.Contains(tt.name, "error") {
				if err := SearchTasklist.Validate(nil, runtime); err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
			}
			out := SearchTasklist.DryRun(nil, runtime).Format()
			for _, want := range tt.wantParts {
				if !strings.Contains(out, want) {
					t.Fatalf("dry run output missing %q: %s", want, out)
				}
			}
		})
	}
}

func TestSearchTasklist_Execute(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		register  func(*httpmock.Registry)
		wantParts []string
	}{
		{
			name: "json success",
			args: []string{"+tasklist-search", "--query", "Q2", "--as", "bot", "--format", "json"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/tasklists/search",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{
							"has_more":   false,
							"page_token": "",
							"items":      []interface{}{map[string]interface{}{"id": "tl-123"}},
						},
					},
				})
				reg.Register(&httpmock.Stub{
					Method: "GET",
					URL:    "/open-apis/task/v2/tasklists/tl-123",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{
							"tasklist": map[string]interface{}{"guid": "tl-123", "name": "Q2 Plan", "url": "https://example.com/tl-123"},
						},
					},
				})
			},
			wantParts: []string{`"guid": "tl-123"`, `"name": "Q2 Plan"`},
		},
		{
			name: "fallback on detail error",
			args: []string{"+tasklist-search", "--query", "fallback", "--as", "bot", "--format", "json"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/tasklists/search",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{
							"has_more":   false,
							"page_token": "",
							"items":      []interface{}{map[string]interface{}{"id": "tl-fallback"}},
						},
					},
				})
				reg.Register(&httpmock.Stub{
					Method: "GET",
					URL:    "/open-apis/task/v2/tasklists/tl-fallback",
					Body:   map[string]interface{}{"code": 99991663, "msg": "not found"},
				})
			},
			wantParts: []string{`"guid": "tl-fallback"`},
		},
		{
			name: "pretty fallback avoids nil name",
			args: []string{"+tasklist-search", "--query", "fallback-pretty", "--as", "bot", "--format", "pretty"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/tasklists/search",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{
							"has_more":   false,
							"page_token": "",
							"items":      []interface{}{map[string]interface{}{"id": "tl-fallback"}},
						},
					},
				})
				reg.Register(&httpmock.Stub{
					Method: "GET",
					URL:    "/open-apis/task/v2/tasklists/tl-fallback",
					Body:   map[string]interface{}{"code": 99991663, "msg": "not found"},
				})
			},
			wantParts: []string{"(unknown tasklist: tl-fallback)", "GUID: tl-fallback"},
		},
		{
			name: "empty pretty with pagination",
			args: []string{"+tasklist-search", "--query", "none", "--as", "bot", "--format", "pretty", "--page-limit", "2"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/tasklists/search",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{"has_more": true, "page_token": "pt_2", "items": []interface{}{}},
					},
				})
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/tasklists/search",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{"has_more": false, "page_token": "", "items": []interface{}{}},
					},
				})
			},
			wantParts: []string{"No tasklists found."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := taskShortcutTestFactory(t)
			warmTenantToken(t, f, reg)
			tt.register(reg)

			s := SearchTasklist
			s.AuthTypes = []string{"bot", "user"}
			err := runMountedTaskShortcut(t, s, tt.args, f, stdout)
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
		})
	}
}
