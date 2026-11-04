// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"encoding/xml"
	"html"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

func TestFormatMessageItemCardLink(t *testing.T) {
	const link = "lark://applink.example.com/client/mail/forward/card?cardId=test-card&ownerId=test-owner&threadId=0"
	rawCard := func(inner string) string {
		encoded, err := json.Marshal(map[string]string{"json_card": inner})
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	const configuredLink = `{"config":{"cardLink":{"url":"` + link + `"}}}`
	for _, tc := range []struct {
		name, msgType, content, want string
	}{
		{"raw card config link", "interactive", rawCard(configuredLink), link},
		{"trim spaces and preserve encoding", "interactive", rawCard(`{"config":{"cardLink":{"url":"  ` + link + `&encoded=a%2Fb%3D  "}}}`), link + "&encoded=a%2Fb%3D"},
		{"missing body content", "interactive", `{}`, ""},
		{"invalid json card type", "interactive", `{"json_card":123}`, ""},
		{"malformed json card", "interactive", rawCard(`{`), ""},
		{"null json card", "interactive", rawCard(`null`), ""},
		{"config missing", "interactive", rawCard(`{}`), ""},
		{"config null", "interactive", rawCard(`{"config":null}`), ""},
		{"config link missing", "interactive", rawCard(`{"config":{}}`), ""},
		{"config link null", "interactive", rawCard(`{"config":{"cardLink":null}}`), ""},
		{"url missing", "interactive", rawCard(`{"config":{"cardLink":{"pc_url":"https://example.com/pc"}}}`), ""},
		{"empty url", "interactive", rawCard(`{"config":{"cardLink":{"url":""}}}`), ""},
		{"blank url", "interactive", rawCard(`{"config":{"cardLink":{"url":" \t"}}}`), ""},
		{"invalid url type", "interactive", rawCard(`{"config":{"cardLink":{"url":123}}}`), ""},
		{"invalid link type", "interactive", rawCard(`{"config":{"cardLink":"invalid"}}`), ""},
		{"element link is not whole-card link", "interactive", rawCard(`{"body":{"elements":[{"url":"` + link + `"}]}}`), ""},
		{"outer card_link is ignored", "interactive", `{"card_link":{"url":"` + link + `"}}`, ""},
		{"malformed content", "interactive", `{`, ""},
		{"non-card message", "text", rawCard(configuredLink), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := map[string]interface{}{
				"message_id": "om_test", "msg_type": tc.msgType,
				"body": map[string]interface{}{"content": tc.content},
			}
			got := FormatMessageItem(raw, nil)
			// Verify the public JSON shape, including omission and URL preservation.
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var decoded map[string]interface{}
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if _, exists := decoded["card_link"]; exists {
				t.Fatalf("unexpected card_link: %s", encoded)
			}
			wantContent := ConvertBodyContent(tc.msgType, &ConvertContext{RawContent: tc.content})
			if tc.want != "" {
				wantContent = strings.TrimSuffix(wantContent, "</card>") + `<cardLinkUrl url="` + html.EscapeString(tc.want) + `"></cardLinkUrl>` + "\n</card>"
			}
			if got["content"] != wantContent {
				t.Fatalf("message content changed: %#v, want %q", got["content"], wantContent)
			}

			root := map[string]interface{}{
				"message_id": "om_root", "msg_type": "merge_forward",
				"body": map[string]interface{}{"content": "Merged forward"},
			}
			children := []map[string]interface{}{raw}
			merged := FormatMessageItemWithMergePrefetch(root, nil, nil, map[string][]map[string]interface{}{"om_root": children})
			encoded, err = json.Marshal(merged)
			if err != nil {
				t.Fatal(err)
			}
			var mergedJSON map[string]interface{}
			if err := json.Unmarshal(encoded, &mergedJSON); err != nil {
				t.Fatal(err)
			}
			if _, exists := mergedJSON["forwarded_card_links"]; exists {
				t.Fatalf("unexpected forwarded_card_links: %s", encoded)
			}
			want := FormatMergeForwardSubTree("om_root", BuildMergeForwardChildrenMap(children, "om_root"))
			if tc.want != "" {
				want = strings.Replace(want, "    </card>", `    <cardLinkUrl url="`+html.EscapeString(tc.want)+`"></cardLinkUrl>`+"\n    </card>", 1)
			}
			if mergedJSON["content"] != want {
				t.Fatalf("merge-forward content = %#v, want %q", mergedJSON["content"], want)
			}
		})
	}
}

func TestFormatMessageItemForwardedCardLinkFetchFallback(t *testing.T) {
	for _, mode := range []string{"inline", "prefetch retry", "fetch failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			var requests atomic.Int32
			rt := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				n := requests.Add(1)
				if req.URL.Path != "/open-apis/im/v1/messages/om_root" {
					t.Fatalf("unexpected request: %s", req.URL)
				}
				if mode == "fetch failure" || (mode == "prefetch retry" && n == 1) {
					return convertlibJSONResponse(http.StatusOK, map[string]interface{}{"code": 2200, "msg": "fetch failed"}), nil
				}
				return convertlibJSONResponse(http.StatusOK, map[string]interface{}{"code": 0, "data": map[string]interface{}{
					"items": []interface{}{map[string]interface{}{
						"message_id": "om_child", "msg_type": "interactive",
						"body": map[string]interface{}{"content": `{"json_card":"{\"config\":{\"cardLink\":{\"url\":\"https://example.com/card\"}}}"}`},
					}},
				}}), nil
			}))
			previous := rt.Factory
			factory, _, _, _ := cmdutil.TestFactory(t, rt.Config)
			factory.HttpClient, factory.LarkClient = previous.HttpClient, previous.LarkClient
			rt.Factory = factory
			root := map[string]interface{}{
				"message_id": "om_root", "msg_type": "merge_forward",
				"body": map[string]interface{}{"content": "Merged forward"},
			}
			var prefetch map[string][]map[string]interface{}
			wantRequests := int32(1)
			if mode == "prefetch retry" {
				prefetch = PrefetchMergeForwardSubItems(rt, []interface{}{root}, nil)
				wantRequests = 2
			}
			got := FormatMessageItemWithMergePrefetch(root, rt, nil, prefetch)
			if requests.Load() != wantRequests {
				t.Fatalf("request count = %d, want %d", requests.Load(), wantRequests)
			}
			if mode == "fetch failure" {
				if _, exists := got["forwarded_card_links"]; exists || !strings.Contains(got["content"].(string), "fetch failed") {
					t.Fatalf("unexpected failed fetch output: %#v", got)
				}
				return
			}
			if _, exists := got["forwarded_card_links"]; exists {
				t.Fatalf("unexpected forwarded_card_links: %#v", got)
			}
			if !strings.Contains(got["content"].(string), "\n    "+`<cardLinkUrl url="https://example.com/card"></cardLinkUrl>`+"\n    </card>") {
				t.Fatalf("missing link in merged content: %#v", got)
			}
		})
	}
}

func TestExpandThreadRepliesPreservesCardLinks(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	const link = "https://example.com/card?thread=a%2Fb%3D"
	card := map[string]interface{}{
		"message_id": "om_card", "msg_type": "interactive",
		"body": map[string]interface{}{"content": `{"json_card":"{\"config\":{\"cardLink\":{\"url\":\"` + link + `\"}}}"}`},
	}
	rt := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("card_msg_content_type") != "raw_card_content" {
			t.Error("thread and merged-forward reads must request raw card content")
		}
		items := []interface{}{card}
		switch req.URL.Path {
		case "/open-apis/im/v1/messages":
			if req.URL.Query().Get("container_id_type") != "thread" || req.URL.Query().Get("container_id") != "omt_test" {
				t.Fatalf("unexpected thread request: %s", req.URL)
			}
			items = append(items, map[string]interface{}{
				"message_id": "om_merge", "msg_type": "merge_forward",
				"body": map[string]interface{}{"content": "Merged forward"},
			})
		case "/open-apis/im/v1/messages/om_merge":
		default:
			t.Fatalf("unexpected request: %s", req.URL)
		}
		return convertlibJSONResponse(http.StatusOK, map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"items": items, "has_more": false,
		}}), nil
	}))
	host := map[string]interface{}{"message_id": "om_root", "thread_id": "omt_test"}
	ExpandThreadReplies(rt, []map[string]interface{}{host}, nil, 50, 500)
	encoded, err := json.Marshal(host)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Replies []struct {
			CardLink json.RawMessage `json:"card_link"`
			Content  string          `json:"content"`
		} `json:"thread_replies"`
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Replies) != 2 || !strings.HasSuffix(got.Replies[0].Content, `<cardLinkUrl url="`+html.EscapeString(link)+`"></cardLinkUrl>`+"\n</card>") {
		t.Fatalf("missing card link in thread reply: %s", encoded)
	}
	for _, reply := range got.Replies {
		if len(reply.CardLink) != 0 {
			t.Fatalf("unexpected card_link in thread reply: %s", encoded)
		}
	}
	if !strings.Contains(got.Replies[1].Content, "\n    "+`<cardLinkUrl url="`+html.EscapeString(link)+`"></cardLinkUrl>`+"\n    </card>") {
		t.Fatalf("missing card link in merged-forward thread reply: %s", encoded)
	}
}

func TestCardLinkURLValidationAndAttributeEscaping(t *testing.T) {
	for _, tc := range []struct{ name, url, want string }{
		{"https", "https://example.com/card", "https://example.com/card"},
		{"http", "http://example.com/card", "http://example.com/card"},
		{"lark", "lark://applink.example.com/card?a=%2f%2F%3D&b=2", "lark://applink.example.com/card?a=%2f%2F%3D&b=2"},
		{"custom app scheme", "feishu://applink.example.com/card", "feishu://applink.example.com/card"},
		{"trim spaces", "  https://example.com/card  ", "https://example.com/card"},
		{"attribute escaping", `https://example.com/card?a="<tag>'&b=%2F%3D`, `https://example.com/card?a="<tag>'&b=%2F%3D`},
		{"plain text", "not-a-url", ""},
		{"no host", "https://", ""},
		{"no scheme", "//example.com", ""},
		{"invalid scheme", "1https://example.com", ""},
		{"script scheme", "javascript://example.com/alert(1)", ""},
		{"data scheme", "data://example.com/payload", ""},
		{"vbscript scheme", "VBScript://example.com/payload", ""},
		{"opaque URL", "https:example.com", ""},
		{"invalid path escape", "https://example.com/%ZZ", ""},
		{"invalid query escape", "https://example.com/?q=%ZZ", ""},
		{"invalid host", "https://exa mple.com/card", ""},
		{"internal space", "https://example.com/a b", ""},
		{"newline", "https://example.com/card\nforged", ""},
		{"leading newline", "\nhttps://example.com/card", ""},
		{"carriage return", "https://example.com/card\r", ""},
		{"tab", "https://example.com/card\t", ""},
		{"control", "https://example.com/card\x00", ""},
		{"unicode line separator", "https://example.com/card\u2028forged", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inner, err := json.Marshal(map[string]interface{}{"config": map[string]interface{}{"cardLink": map[string]string{"url": tc.url}}})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(map[string]string{"json_card": string(inner)})
			if err != nil {
				t.Fatal(err)
			}
			item := map[string]interface{}{"msg_type": "interactive", "body": map[string]interface{}{"content": string(raw)}}
			type renderedCard struct {
				XMLName xml.Name `xml:"card"`
				Links   []struct {
					URL string `xml:"url,attr"`
				} `xml:"cardLinkUrl"`
			}
			root := map[string]interface{}{
				"message_id": "om_root", "msg_type": "merge_forward",
				"body": map[string]interface{}{"content": "Merged forward"},
			}
			merged := FormatMessageItemWithMergePrefetch(root, nil, nil, map[string][]map[string]interface{}{"om_root": {item}})
			for name, content := range map[string]string{
				"direct":    FormatMessageItem(item, nil)["content"].(string),
				"forwarded": merged["content"].(string),
			} {
				var card renderedCard
				if name == "direct" {
					if err := xml.Unmarshal([]byte(content), &card); err != nil {
						t.Fatalf("invalid card markup: %v: %s", err, content)
					}
				} else {
					var forward struct {
						Cards []renderedCard `xml:"card"`
					}
					if err := xml.Unmarshal([]byte(content), &forward); err != nil || len(forward.Cards) != 1 {
						t.Fatalf("invalid forwarded card markup: %v: %s", err, content)
					}
					card = forward.Cards[0]
				}
				if tc.want == "" {
					if len(card.Links) != 0 {
						t.Fatalf("unexpected %s link: %s", name, content)
					}
				} else if len(card.Links) != 1 || card.Links[0].URL != tc.want {
					t.Fatalf("decoded %s link = %+v, want %q: %s", name, card.Links, tc.want, content)
				}
			}
		})
	}
}
