// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Completion struct {
	Status         string `json:"status"`
	RequestedCount int    `json:"requested_count"`
	SucceededCount int    `json:"succeeded_count"`
	FailedCount    int    `json:"failed_count"`
	PendingCount   int    `json:"pending_count"`
	SucceededItems []any  `json:"succeeded_items"`
	FailedItems    []any  `json:"failed_items"`
	PendingItems   []any  `json:"pending_items"`
	RetryScope     string `json:"retry_scope"`
}

type ledgerItem struct {
	key   string
	value any
}

type extraction struct {
	items         []ledgerItem
	rawCount      int
	selectedCount int
	rejectedCount int
	present       bool
}

func extract(root map[string]any, spec evidenceSpec) extraction {
	if root == nil || spec.Field == "" {
		return extraction{}
	}
	raw, present := root[spec.Field]
	if !present {
		return extraction{}
	}
	values, ok := raw.([]any)
	out := extraction{present: true}
	if !ok {
		out.rejectedCount = 1
		return out
	}
	out.rawCount = len(values)
	for _, value := range values {
		item, ok := extractItem(value, spec)
		if !ok {
			out.rejectedCount++
			continue
		}
		out.selectedCount++
		out.items = append(out.items, item)
	}
	out.items = uniqueItems(out.items)
	return out
}

func extractItem(value any, spec evidenceSpec) (ledgerItem, bool) {
	switch spec.Shape {
	case evidenceStrings:
		return stringItem(value)
	case evidenceObjects:
		object, ok := value.(map[string]any)
		if !ok {
			return ledgerItem{}, false
		}
		return stringItem(object[spec.IDField])
	case evidenceNestedObjects:
		object, ok := nestedObject(value, spec.Container)
		if !ok {
			return ledgerItem{}, false
		}
		return stringItem(object[spec.IDField])
	case evidenceFeedObjects:
		object, ok := value.(map[string]any)
		if !ok {
			return ledgerItem{}, false
		}
		return feedItem(object)
	case evidenceNestedFeedObjects:
		object, ok := nestedObject(value, spec.Container)
		if !ok {
			return ledgerItem{}, false
		}
		return feedItem(object)
	case evidenceStatusObjects:
		object, ok := value.(map[string]any)
		if !ok {
			return ledgerItem{}, false
		}
		status := nonEmptyString(object["status"])
		if status != "ok" && status != "failed" {
			return ledgerItem{}, false
		}
		return stringItem(object[spec.IDField])
	default:
		return ledgerItem{}, false
	}
}

func nestedObject(value any, field string) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	nested, ok := object[field].(map[string]any)
	return nested, ok
}

func stringItem(value any) (ledgerItem, bool) {
	id := stableID(value)
	if id == "" {
		return ledgerItem{}, false
	}
	return ledgerItem{key: id, value: id}, true
}

func feedItem(object map[string]any) (ledgerItem, bool) {
	feedID := stableID(object["feed_id"])
	feedType := stableID(object["feed_type"])
	if feedID == "" || feedType == "" {
		return ledgerItem{}, false
	}
	return ledgerItem{
		key: feedType + "\x00" + feedID,
		value: map[string]any{
			"feed_id": feedID, "feed_type": feedType,
		},
	}, true
}

func nonEmptyString(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func stableID(value any) string {
	switch id := value.(type) {
	case string:
		return strings.TrimSpace(id)
	case json.Number:
		return string(id)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(id)
	default:
		return ""
	}
}

func uniqueItems(items []ledgerItem) []ledgerItem {
	out := make([]ledgerItem, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.key == "" {
			continue
		}
		if _, ok := seen[item.key]; ok {
			continue
		}
		seen[item.key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func completion(requested, failed, pending []ledgerItem, recovery PartialRecoveryMode) Completion {
	requested = uniqueItems(requested)
	requestedSet := make(map[string]struct{}, len(requested))
	for _, item := range requested {
		requestedSet[item.key] = struct{}{}
	}
	filterRequested := func(items []ledgerItem, excluded map[string]struct{}) []ledgerItem {
		out := make([]ledgerItem, 0, len(items))
		for _, item := range uniqueItems(items) {
			if _, ok := requestedSet[item.key]; !ok {
				continue
			}
			if _, blocked := excluded[item.key]; blocked {
				continue
			}
			out = append(out, item)
		}
		return out
	}

	// A contradictory pending+failed response is treated as pending. Pending
	// means the final state is unknown, so authorizing a retry would be unsafe.
	pending = filterRequested(pending, nil)
	pendingSet := make(map[string]struct{}, len(pending))
	for _, item := range pending {
		pendingSet[item.key] = struct{}{}
	}
	failed = filterRequested(failed, pendingSet)
	blocked := make(map[string]struct{}, len(failed)+len(pending))
	for key := range pendingSet {
		blocked[key] = struct{}{}
	}
	for _, item := range failed {
		blocked[item.key] = struct{}{}
	}
	succeeded := make([]ledgerItem, 0, len(requested))
	for _, item := range requested {
		if _, exists := blocked[item.key]; !exists {
			succeeded = append(succeeded, item)
		}
	}
	status := "complete"
	retryScope := "none"
	if len(failed) > 0 || len(pending) > 0 {
		status = "partial"
		switch {
		case len(pending) > 0:
			retryScope = "none"
		case recovery == PartialRecoveryWholeRequest:
			retryScope = "whole_request"
		default:
			retryScope = "failed_items_only"
		}
	}
	values := func(items []ledgerItem) []any {
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, item.value)
		}
		return out
	}
	return Completion{
		Status:         status,
		RequestedCount: len(requested),
		SucceededCount: len(succeeded),
		FailedCount:    len(failed),
		PendingCount:   len(pending),
		SucceededItems: values(succeeded),
		FailedItems:    values(failed),
		PendingItems:   values(pending),
		RetryScope:     retryScope,
	}
}
