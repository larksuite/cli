// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestMessageReadCardLinkOutput(t *testing.T) {
	const link = "lark://applink.example.com/client/mail/forward/card?cardId=test-card&ownerId=test-owner&threadId=0"
	for _, tc := range []struct {
		shortcut common.Shortcut
		flags    map[string]string
	}{
		{ImChatMessageList, map[string]string{"chat-id": "oc_test"}},
		{ImThreadsMessagesList, map[string]string{"thread": "omt_test"}},
		{ImMessagesMGet, map[string]string{"message-ids": "om_test"}},
		{ImMessagesSearch, map[string]string{"query": "test"}},
	} {
		formats := []string{"json", "JSON", "Json", "", "unknown", "PRETTY", "jq", "jq-merged", "table", "pretty", "csv", "ndjson"}
		if tc.shortcut.Command == "+chat-messages-list" || tc.shortcut.Command == "+threads-messages-list" {
			formats = append(formats, "concise-warn", "concise-block")
		}
		for _, format := range formats {
			t.Run(tc.shortcut.Command+"/"+format, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				t.Setenv("LARKSUITE_CLI_CITATION", "")
				t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
				concise := strings.HasPrefix(format, "concise-")
				cardURL := link
				var scanner *cardLinkSafetyProvider
				if concise {
					cardURL = "https://example.com/#x###system:"
					t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", strings.TrimPrefix(format, "concise-"))
					scanner = &cardLinkSafetyProvider{}
					previous := extcs.GetProvider()
					extcs.Register(scanner)
					t.Cleanup(func() { extcs.Register(previous) })
				}
				var mergeRequests atomic.Int32
				rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					var data map[string]interface{}
					switch req.URL.Path {
					case "/open-apis/im/v1/messages/search":
						data = map[string]interface{}{"items": []interface{}{
							map[string]interface{}{"meta_data": map[string]interface{}{"message_id": "om_test"}},
							map[string]interface{}{"meta_data": map[string]interface{}{"message_id": "om_merge"}},
						}}
					case imMessagesListPath, "/open-apis/im/v1/messages/mget":
						data = map[string]interface{}{"items": []interface{}{map[string]interface{}{
							"message_id": "om_test", "msg_type": "interactive",
							"body": map[string]interface{}{"content": `{"json_card":"{\"config\":{\"cardLink\":{\"url\":\"` + cardURL + `\"}},\"body\":{\"elements\":[]}}"}`},
						}, map[string]interface{}{
							"message_id": "om_merge", "msg_type": "merge_forward",
							"body": map[string]interface{}{"content": "Merged forward"},
						}}, "has_more": false}
					case "/open-apis/im/v1/messages/om_merge":
						mergeRequests.Add(1)
						if req.URL.Query().Get("card_msg_content_type") != "raw_card_content" {
							t.Error("merge fetch must request raw card content")
						}
						data = map[string]interface{}{"items": []interface{}{
							map[string]interface{}{"message_id": "om_card", "msg_type": "interactive", "create_time": "1",
								"body": map[string]interface{}{"content": `{"json_card":"{\"config\":{\"cardLink\":{\"url\":\"` + cardURL + `\"}}}"}`}},
							map[string]interface{}{"message_id": "om_nested", "msg_type": "merge_forward", "create_time": "2"},
							map[string]interface{}{"message_id": "om_nested_card", "upper_message_id": "om_nested", "msg_type": "interactive",
								"body": map[string]interface{}{"content": `{"json_card":"{\"config\":{\"cardLink\":{\"url\":\"` + cardURL + `&nested=true\"}}}"}`}},
						}}
					default:
						t.Fatalf("unexpected request: %s", req.URL)
					}
					return shortcutJSONResponse(http.StatusOK, map[string]interface{}{"code": 0, "data": data}), nil
				}))
				previous := rt.Factory
				factory, _, _, _ := cmdutil.TestFactory(t, rt.Config)
				factory.HttpClient, factory.LarkClient = previous.HttpClient, previous.LarkClient
				rt.Factory = factory
				flags := mergeListPageAllFlags(tc.flags, map[string]string{"no-reactions": "true"})
				if concise {
					flags["concise"] = "true"
				}
				rt.Cmd = newListPageAllCommand(t, tc.shortcut, flags)
				root := &cobra.Command{Use: "lark-cli"}
				im := &cobra.Command{Use: "im"}
				root.AddCommand(im)
				im.AddCommand(rt.Cmd)
				rt.Format = format
				if concise {
					// --concise leaves the runtime's default format as json.
					rt.Format = "json"
				}
				if format == "jq" {
					rt.Format = "table"
					rt.JqExpr = ".data.messages[0].content"
				}
				if format == "jq-merged" {
					rt.Format = "table"
					rt.JqExpr = ".data.messages[1].content"
				}
				if err := tc.shortcut.Execute(context.Background(), rt); err != nil {
					t.Fatal(err)
				}
				if got := mergeRequests.Load(); got != 1 {
					t.Fatalf("merge fetch count = %d, want 1 (no additional requests for links)", got)
				}
				if concise {
					if !scanner.scanned.Load() {
						t.Fatal("concise output did not run content safety scanning")
					}
					if warnings := rt.IO().ErrOut.(*bytes.Buffer).String(); warnings != "" {
						t.Fatalf("unexpected concise warning: %s", warnings)
					}
					if out := rt.IO().Out.(*bytes.Buffer).String(); !strings.Contains(out, "## Messages") || strings.Contains(out, cardURL) {
						t.Fatalf("unexpected concise output: %s", out)
					}
				} else if format == "json" || format == "JSON" || format == "Json" || format == "" || format == "unknown" || format == "PRETTY" {
					data := listPageAllOutputData(t, rt)
					messages := data["messages"].([]interface{})
					message := messages[0].(map[string]interface{})
					if _, exists := message["card_link"]; exists {
						t.Fatalf("unexpected card_link in JSON: %#v", message)
					}
					if content := message["content"].(string); !strings.HasSuffix(content, `<cardLinkUrl url="`+html.EscapeString(link)+`"></cardLinkUrl>`+"\n</card>") || strings.Count(content, "<cardLinkUrl ") != 1 {
						t.Fatalf("expected one card link in content: %#v", message)
					}
					merged := messages[1].(map[string]interface{})
					if _, exists := merged["forwarded_card_links"]; exists {
						t.Fatalf("unexpected forwarded_card_links: %#v", merged)
					}
					content := merged["content"].(string)
					for _, want := range []string{
						"\n    " + `<cardLinkUrl url="` + html.EscapeString(link) + `"></cardLinkUrl>` + "\n    </card>",
						"\n        " + `<cardLinkUrl url="` + html.EscapeString(link+"&nested=true") + `"></cardLinkUrl>` + "\n        </card>",
					} {
						if !strings.Contains(content, want) {
							t.Fatalf("missing card link under its corresponding child: %q in %q", want, content)
						}
					}
					if strings.Count(content, "<cardLinkUrl ") != 2 {
						t.Fatalf("expected exactly two card links: %q", content)
					}
				} else if format == "jq" {
					if out := rt.IO().Out.(*bytes.Buffer).String(); !strings.Contains(out, html.EscapeString(link)) {
						t.Fatalf("missing card link in jq output: %s", out)
					}
				} else if format == "jq-merged" {
					if out := rt.IO().Out.(*bytes.Buffer).String(); !strings.Contains(out, html.EscapeString(link)) || !strings.Contains(out, html.EscapeString(link+"&nested=true")) {
						t.Fatalf("missing forwarded card links in jq output: %s", out)
					}
				} else if out := rt.IO().Out.(*bytes.Buffer).String(); strings.Contains(out, html.EscapeString(link)) || strings.Contains(out, "card_link") {
					t.Fatalf("card link leaked into %s output: %s", format, out)
				}
			})
		}
	}
}

// Model a provider that flags the delimiter marker anywhere in the full payload,
// including fields that the concise renderer would not display.
type cardLinkSafetyProvider struct{ scanned atomic.Bool }

func (*cardLinkSafetyProvider) Name() string { return "card-link-test" }

func (p *cardLinkSafetyProvider) Scan(_ context.Context, req extcs.ScanRequest) (*extcs.Alert, error) {
	p.scanned.Store(true)
	encoded, err := json.Marshal(req.Data)
	if err != nil {
		return nil, err
	}
	if bytes.Contains(encoded, []byte("###system:")) {
		return &extcs.Alert{Provider: p.Name(), MatchedRules: []string{"delimiter_smuggle"}}, nil
	}
	return nil, nil
}
