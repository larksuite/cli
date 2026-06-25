// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// TestImChatSearchExecutePassesThroughNotice verifies chat search notice output.
func TestImChatSearchExecutePassesThroughNotice(t *testing.T) {
	const notice = "The query is too long and has been truncated to the first 50 characters for search."
	longQuery := strings.Repeat("q", 81)

	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/open-apis/im/v2/chats/search") {
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
		var body map[string]interface{}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return nil, fmt.Errorf("decode request body: %w", err)
		}
		if got, _ := body["query"].(string); got != longQuery {
			return nil, fmt.Errorf("body.query = %q, want %q", got, longQuery)
		}
		return shortcutJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"notice":     notice,
				"items":      []interface{}{},
				"total":      0,
				"has_more":   false,
				"page_token": "",
			},
		}), nil
	}))
	runtime.Cmd = newChatSearchNoticeTestCommand(t, longQuery)
	runtime.Format = "json"

	if err := ImChatSearch.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("ImChatSearch.Execute() error = %v", err)
	}

	data := decodeShortcutData(t, runtime)
	if got, _ := data["notice"].(string); got != notice {
		t.Fatalf("data.notice = %q, want %q; data=%#v", got, notice, data)
	}
}

// TestImMessagesSearchExecutePassesThroughNotice verifies message search notice output.
func TestImMessagesSearchExecutePassesThroughNotice(t *testing.T) {
	const notice = "The query is too long and has been truncated to the first 50 characters for search."

	runtime := newMessagesSearchRuntime(t, map[string]string{
		"query": "incident",
	}, nil, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/search") {
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
		return shortcutJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"notice":     notice,
				"items":      []interface{}{},
				"has_more":   false,
				"page_token": "",
			},
		}), nil
	}))
	runtime.Format = "json"

	if err := ImMessagesSearch.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("ImMessagesSearch.Execute() error = %v", err)
	}

	data := decodeShortcutData(t, runtime)
	if got, _ := data["notice"].(string); got != notice {
		t.Fatalf("data.notice = %q, want %q; data=%#v", got, notice, data)
	}
}

// newChatSearchNoticeTestCommand builds a typed chat-search command for notice tests.
func newChatSearchNoticeTestCommand(t *testing.T, query string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	for _, name := range []string{"query", "search-types", "member-ids", "sort-by", "page-token"} {
		cmd.Flags().String(name, "", "")
	}
	for _, name := range []string{"is-manager", "disable-search-by-user", "exclude-muted"} {
		cmd.Flags().Bool(name, false, "")
	}
	cmd.Flags().Int("page-size", 20, "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if err := cmd.Flags().Set("query", query); err != nil {
		t.Fatalf("Flags().Set(query) error = %v", err)
	}
	return cmd
}

// decodeShortcutData extracts the JSON envelope data object from shortcut output.
func decodeShortcutData(t *testing.T, runtime *common.RuntimeContext) map[string]interface{} {
	t.Helper()

	out, ok := runtime.Factory.IOStreams.Out.(*bytes.Buffer)
	if !ok {
		t.Fatalf("stdout buffer has type %T", runtime.Factory.IOStreams.Out)
	}
	var env map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\nstdout=%s", err, out.String())
	}
	data, ok := env["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("envelope data missing or wrong type: %#v", env)
	}
	return data
}
