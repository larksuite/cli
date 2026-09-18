// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Page.Items is declared required;nonnullable, so a zero-item collection must
// encode as [] -- a caller generating types from the published schema would
// reject null, and an empty result page is an ordinary outcome, not an error.
func TestEmptyPageEncodesAsArrayNotNull(t *testing.T) {
	host := NewCommandContext(ContextOptions{
		CollectPages: func(context.Context, Request, bool) ([]map[string]any, HostPagination, error) {
			return []map[string]any{{"items": []any{}, "has_more": false}}, HostPagination{Complete: true, Pages: 1}, nil
		},
	})
	page, err := CollectPages[string](context.Background(), host, GET("/open-apis/im/v1/chats"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "null") {
		t.Fatalf("empty page = %s", encoded)
	}
	if string(encoded) != `{"items":[]}` {
		t.Fatalf("empty page = %s", encoded)
	}
}

// The same has to hold when the walk fails partway: the caller still receives a
// Page, and its Items must satisfy the published schema.
func TestFailedPageCollectionStillEncodesItemsAsArray(t *testing.T) {
	host := NewCommandContext(ContextOptions{DryRun: true})
	page, err := CollectPages[string](context.Background(), host, GET("/open-apis/im/v1/chats"))
	if err == nil {
		t.Fatal("dry-run page collection returned no error")
	}
	encoded, marshalErr := json.Marshal(page)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if string(encoded) != `{"items":[]}` {
		t.Fatalf("failed page = %s", encoded)
	}
}
