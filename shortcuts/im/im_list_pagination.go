// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

const imMessagesListPath = "/open-apis/im/v1/messages"

const pageAllDryRunDescription = "Auto-paginates from --page-token when provided, until exhaustion or --page-limit is reached"

// imMapListPage is the page shape shared by IM endpoints whose items are JSON
// objects. Endpoint-specific shapes such as chat search stay in their command.
type imMapListPage struct {
	Items         []map[string]interface{} `json:"items"`
	HasMore       bool                     `json:"has_more"`
	PageToken     string                   `json:"page_token"`
	NextPageToken string                   `json:"next_page_token"`
}

type imMapListResult struct {
	items     []map[string]interface{}
	hasMore   bool
	pageToken string
}

func (result *imMapListResult) AddPage(page imMapListPage) error {
	for _, item := range page.Items {
		if item != nil {
			result.items = append(result.items, item)
		}
	}
	result.hasMore = page.HasMore
	result.pageToken = page.PageToken
	if result.pageToken == "" {
		result.pageToken = page.NextPageToken
	}
	return nil
}

// interfaceItems adapts typed items only at the legacy merge-forward boundary.
func (result *imMapListResult) interfaceItems() []interface{} {
	items := make([]interface{}, len(result.items))
	for i, item := range result.items {
		items[i] = item
	}
	return items
}

// messageListPageParams preserves the SDK query map's repeated-value shape for
// the shared paginator.
func messageListPageParams(params map[string][]string) map[string]interface{} {
	out := make(map[string]interface{}, len(params))
	for name, values := range params {
		out[name] = append([]string(nil), values...)
	}
	return out
}
