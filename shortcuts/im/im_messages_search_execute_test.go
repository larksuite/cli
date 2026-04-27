// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newMessagesSearchRuntime(t *testing.T, stringFlags map[string]string, boolFlags map[string]bool, rt http.RoundTripper) *common.RuntimeContext {
	t.Helper()

	runtime := newBotShortcutRuntime(t, rt)
	cmd := &cobra.Command{Use: "test"}

	stringFlagNames := []string{"query", "page-token", "at-chatter-ids"}
	for _, name := range stringFlagNames {
		cmd.Flags().String(name, "", "")
	}
	cmd.Flags().Int("page-size", 20, "")
	cmd.Flags().Int("page-limit", 20, "")
	boolFlagNames := []string{"page-all"}
	for _, name := range boolFlagNames {
		cmd.Flags().Bool(name, false, "")
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	for name, value := range stringFlags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	for name, value := range boolFlags {
		if err := cmd.Flags().Set(name, map[bool]string{true: "true", false: "false"}[value]); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	runtime.Cmd = cmd
	runtime.Format = "pretty"
	return runtime
}

func TestImMessagesSearchExecuteAutoPaginationBatches(t *testing.T) {
	var (
		searchPageTokens []string
		mgetBatchSizes   []int
		chatBatchSizes   []int
	)

	runtime := newMessagesSearchRuntime(t, map[string]string{
		"query":      "incident",
		"page-limit": "2",
	}, map[string]bool{
		"page-all": true,
	}, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/search"):
			pageToken := req.URL.Query().Get("page_token")
			searchPageTokens = append(searchPageTokens, pageToken)
			switch pageToken {
			case "":
				return shortcutJSONResponse(200, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"items":      buildSearchResultItems(1, 50),
						"has_more":   true,
						"page_token": "tok_p2",
					},
				}), nil
			case "tok_p2":
				return shortcutJSONResponse(200, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"items":      buildSearchResultItems(51, 55),
						"has_more":   true,
						"page_token": "tok_p3",
					},
				}), nil
			default:
				return nil, fmt.Errorf("unexpected search page_token: %q", pageToken)
			}
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/mget"):
			ids := req.URL.Query()["message_ids"]
			mgetBatchSizes = append(mgetBatchSizes, len(ids))
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"items": buildMessageDetails(ids),
				},
			}), nil
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/batch_query"):
			var body struct {
				ChatIDs []string `json:"chat_ids"`
			}
			rawBody, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if err := json.Unmarshal(rawBody, &body); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			chatBatchSizes = append(chatBatchSizes, len(body.ChatIDs))
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"items": buildChatContexts(body.ChatIDs),
				},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	if err := ImMessagesSearch.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("ImMessagesSearch.Execute() error = %v", err)
	}

	if !reflect.DeepEqual(searchPageTokens, []string{"", "tok_p2"}) {
		t.Fatalf("search page tokens = %#v, want %#v", searchPageTokens, []string{"", "tok_p2"})
	}
	if !reflect.DeepEqual(mgetBatchSizes, []int{50, 5}) {
		t.Fatalf("mget batch sizes = %#v, want %#v", mgetBatchSizes, []int{50, 5})
	}
	if !reflect.DeepEqual(chatBatchSizes, []int{50, 5}) {
		t.Fatalf("chat batch sizes = %#v, want %#v", chatBatchSizes, []int{50, 5})
	}

	outBuf, _ := runtime.Factory.IOStreams.Out.(*bytes.Buffer)
	if outBuf == nil {
		t.Fatal("stdout buffer missing")
	}
	output := outBuf.String()
	if !strings.Contains(output, "55 search result(s)") {
		t.Fatalf("stdout = %q, want search results summary", output)
	}
	if !strings.Contains(output, "warning: stopped after fetching 2 page(s)") {
		t.Fatalf("stdout = %q, want page limit warning", output)
	}
}

func TestImMessagesSearchExecuteExplicitPageLimitWithoutPageAll(t *testing.T) {
	var searchCalls int

	runtime := newMessagesSearchRuntime(t, map[string]string{
		"query":      "incident",
		"page-limit": "2",
	}, nil, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/search"):
			searchCalls++
			pageToken := req.URL.Query().Get("page_token")
			if searchCalls == 1 {
				if pageToken != "" {
					return nil, fmt.Errorf("unexpected first page token: %q", pageToken)
				}
				return shortcutJSONResponse(200, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"items":      buildSearchResultItems(1, 1),
						"has_more":   true,
						"page_token": "tok_p2",
					},
				}), nil
			}
			if pageToken != "tok_p2" {
				return nil, fmt.Errorf("unexpected second page token: %q", pageToken)
			}
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"items":      buildSearchResultItems(2, 2),
					"has_more":   false,
					"page_token": "",
				},
			}), nil
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/mget"):
			ids := req.URL.Query()["message_ids"]
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"items": buildMessageDetails(ids),
				},
			}), nil
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/batch_query"):
			var body struct {
				ChatIDs []string `json:"chat_ids"`
			}
			rawBody, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if err := json.Unmarshal(rawBody, &body); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"items": buildChatContexts(body.ChatIDs),
				},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	if err := ImMessagesSearch.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("ImMessagesSearch.Execute() error = %v", err)
	}
	if searchCalls != 2 {
		t.Fatalf("searchCalls = %d, want 2", searchCalls)
	}
}

func buildSearchResultItems(start, end int) []interface{} {
	items := make([]interface{}, 0, end-start+1)
	for i := start; i <= end; i++ {
		items = append(items, map[string]interface{}{
			"meta_data": map[string]interface{}{
				"message_id": fmt.Sprintf("om_%03d", i),
			},
		})
	}
	return items
}

func buildMessageDetails(ids []string) []interface{} {
	items := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		suffix := strings.TrimPrefix(id, "om_")
		items = append(items, map[string]interface{}{
			"message_id":  id,
			"msg_type":    "text",
			"create_time": "1710000000",
			"chat_id":     "oc_" + suffix,
			"sender": map[string]interface{}{
				"id":          "cli_bot",
				"name":        "Bot",
				"sender_type": "bot",
			},
			"body": map[string]interface{}{
				"content": fmt.Sprintf(`{"text":"message %s"}`, suffix),
			},
		})
	}
	return items
}

func buildChatContexts(chatIDs []string) []interface{} {
	items := make([]interface{}, 0, len(chatIDs))
	for _, chatID := range chatIDs {
		items = append(items, map[string]interface{}{
			"chat_id":   chatID,
			"chat_mode": "group",
			"name":      "Chat " + strings.TrimPrefix(chatID, "oc_"),
		})
	}
	return items
}
