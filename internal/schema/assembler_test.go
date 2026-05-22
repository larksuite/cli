// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"reflect"
	"testing"
)

func TestKeyOrderIndex_ImReactionsList(t *testing.T) {
	// im.reactions.list 在 meta_data.json 里 parameters 顺序是固定的：
	// message_id (path), reaction_type, page_token, page_size, user_id_type (all query)
	order := lookupKeyOrder("im", []string{"reactions"}, "list")
	if order == nil {
		t.Fatal("expected key order for im.reactions.list, got nil")
	}
	wantParameters := []string{"message_id", "reaction_type", "page_token", "page_size", "user_id_type"}
	if !reflect.DeepEqual(order.Parameters, wantParameters) {
		t.Errorf("parameters order:\ngot:  %v\nwant: %v", order.Parameters, wantParameters)
	}
	// im.reactions.list 没有 requestBody（GET 方法）
	if len(order.RequestBody) != 0 {
		t.Errorf("expected empty RequestBody, got %v", order.RequestBody)
	}
}

func TestKeyOrderIndex_ImImagesCreate(t *testing.T) {
	// im.images.create 在 meta_data.json 里 requestBody 顺序是：image_type, image
	order := lookupKeyOrder("im", []string{"images"}, "create")
	if order == nil {
		t.Fatal("expected key order for im.images.create, got nil")
	}
	wantBody := []string{"image_type", "image"}
	if !reflect.DeepEqual(order.RequestBody, wantBody) {
		t.Errorf("requestBody order:\ngot:  %v\nwant: %v", order.RequestBody, wantBody)
	}
}

func TestKeyOrderIndex_UnknownPath(t *testing.T) {
	// 远端缓存的命令（不在 embedded 内）查不到 key order，返回 nil 走字母序兜底
	order := lookupKeyOrder("nonexistent_service", []string{"foo"}, "bar")
	if order != nil {
		t.Errorf("expected nil for unknown path, got %+v", order)
	}
}
