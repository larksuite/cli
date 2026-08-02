// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"net/http"
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
