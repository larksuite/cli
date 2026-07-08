// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"strings"
	"testing"
)

func TestMatchCommandItem(t *testing.T) {
	items := []interface{}{
		sampleItem("greet", "id1"),
		sampleItem("weather", "id2"),
	}
	id, item := matchCommandItem(items, "weather")
	if id != "id2" || item == nil {
		t.Fatalf("got id=%q item=%v", id, item)
	}
	id, item = matchCommandItem(items, "nope")
	if id != "" || item != nil {
		t.Fatalf("miss should return empty, got id=%q", id)
	}
	// 精确匹配：大小写与空白不做宽容
	id, _ = matchCommandItem(items, "Greet")
	if id != "" {
		t.Fatalf("match must be exact, got %q", id)
	}
}

func TestResolveNotFoundErrorShape(t *testing.T) {
	err := commandNotFoundError("nope")
	if err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err should say not found: %v", err)
	}
}
