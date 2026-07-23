// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestImChatSearchExecuteTotalContract(t *testing.T) {
	tests := []struct {
		name      string
		total     interface{}
		wantTotal bool
		wantValue int
	}{
		{name: "missing"},
		{name: "invalid", total: "unknown"},
		{name: "zero", total: 0},
		{name: "fractional truncates to zero", total: 0.5},
		{name: "positive", total: 7, wantTotal: true, wantValue: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"meta_data": map[string]interface{}{
							"chat_id": "oc_example",
							"name":    "Example chat",
						},
					},
				},
				"has_more":   false,
				"page_token": "",
			}
			if tt.total != nil {
				data["total"] = tt.total
			}

			runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/open-apis/im/v2/chats/search" {
					return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
				}
				return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
					"code": 0,
					"data": data,
				}), nil
			}))
			runtime.Cmd = newChatSearchNoticeTestCommand(t, "example")
			runtime.Format = "json"

			if err := ImChatSearch.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("ImChatSearch.Execute() error = %v", err)
			}

			got := decodeShortcutData(t, runtime)
			total, exists := got["total"]
			if exists != tt.wantTotal {
				t.Fatalf("data total presence = %v, want %v; data=%#v", exists, tt.wantTotal, got)
			}
			if tt.wantTotal && int(total.(float64)) != tt.wantValue {
				t.Fatalf("data.total = %v, want %d", total, tt.wantValue)
			}
		})
	}
}

func TestImChatSearchExecutePrettyTotal(t *testing.T) {
	tests := []struct {
		name       string
		total      interface{}
		wantFooter string
	}{
		{name: "missing uses displayed count", wantFooter: "1 chat(s) found"},
		{name: "zero uses displayed count", total: 0, wantFooter: "1 chat(s) found"},
		{name: "fractional uses displayed count", total: 0.5, wantFooter: "1 chat(s) found"},
		{name: "positive uses server total", total: 7, wantFooter: "7 chat(s) found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"meta_data": map[string]interface{}{
							"chat_id": "oc_example",
							"name":    "Example chat",
						},
					},
				},
				"has_more":   false,
				"page_token": "",
			}
			if tt.total != nil {
				data["total"] = tt.total
			}

			runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
					"code": 0,
					"data": data,
				}), nil
			}))
			runtime.Cmd = newChatSearchNoticeTestCommand(t, "example")
			runtime.Format = "pretty"

			if err := ImChatSearch.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("ImChatSearch.Execute() error = %v", err)
			}

			out, ok := runtime.Factory.IOStreams.Out.(*bytes.Buffer)
			if !ok {
				t.Fatalf("stdout buffer has type %T", runtime.Factory.IOStreams.Out)
			}
			if !strings.Contains(out.String(), tt.wantFooter) {
				t.Fatalf("pretty output missing %q:\n%s", tt.wantFooter, out.String())
			}
			if strings.Contains(out.String(), "0 chat(s) found") {
				t.Fatalf("pretty output reported a zero count despite displaying a row:\n%s", out.String())
			}
		})
	}
}
