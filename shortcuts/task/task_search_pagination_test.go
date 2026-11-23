// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestSearchPaginationUsesQueryToken(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		command  string
		url      string
	}{
		{
			name:     "tasks",
			shortcut: SearchTask,
			command:  "+search",
			url:      "/open-apis/task/v2/tasks/search",
		},
		{
			name:     "tasklists",
			shortcut: SearchTasklist,
			command:  "+tasklist-search",
			url:      "/open-apis/task/v2/tasklists/search",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := taskShortcutTestFactory(t)
			warmTenantToken(t, f, reg)

			var pageTokens []string
			reg.Register(searchPaginationStub(t, tt.url, "next_pt", true, &pageTokens))
			reg.Register(searchPaginationStub(t, tt.url, "", false, &pageTokens))

			shortcut := tt.shortcut
			shortcut.AuthTypes = []string{"bot", "user"}
			err := runMountedTaskShortcut(t, shortcut, []string{
				tt.command,
				"--query", "pagination",
				"--page-token", "initial_pt",
				"--page-limit", "2",
				"--as", "bot",
				"--format", "json",
			}, f, stdout)
			if err != nil {
				t.Fatalf("search command failed: %v", err)
			}

			want := []string{"initial_pt", "next_pt"}
			if !reflect.DeepEqual(pageTokens, want) {
				t.Fatalf("search page tokens = %#v, want %#v", pageTokens, want)
			}
		})
	}
}

func assertSearchDryRunPageToken(t *testing.T, preview *common.DryRunAPI, want string) {
	t.Helper()

	data, err := preview.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal search dry-run preview: %v", err)
	}
	var envelope struct {
		API []struct {
			Params map[string]interface{} `json:"params"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode search dry-run preview: %v", err)
	}
	if len(envelope.API) != 1 {
		t.Fatalf("search dry-run API call count = %d, want 1; preview = %s", len(envelope.API), data)
	}
	call := envelope.API[0]
	if got, _ := call.Params["page_token"].(string); got != want {
		t.Fatalf("search dry-run params.page_token = %q, want %q; preview = %s", got, want, data)
	}
	if _, present := call.Body["page_token"]; present {
		t.Fatalf("search dry-run body unexpectedly contains page_token; preview = %s", data)
	}
}

func searchPaginationStub(t *testing.T, endpoint, responseToken string, hasMore bool, capturedTokens *[]string) *httpmock.Stub {
	t.Helper()
	return &httpmock.Stub{
		Method: http.MethodPost,
		URL:    endpoint,
		OnMatch: func(req *http.Request) {
			*capturedTokens = append(*capturedTokens, req.URL.Query().Get("page_token"))

			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Errorf("read search request body: %v", err)
				return
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Errorf("decode search request body: %v", err)
				return
			}
			if _, present := payload["page_token"]; present {
				t.Errorf("search request body unexpectedly contains page_token: %s", body)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"has_more":   hasMore,
				"page_token": responseToken,
				"items":      []interface{}{},
			},
		},
	}
}
