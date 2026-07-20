// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestMatchCommandID(t *testing.T) {
	items := []interface{}{
		sampleItem("greet", "id1"),
		sampleItem("weather", "id2"),
	}
	id := matchCommandID(items, "weather")
	if id != "id2" {
		t.Fatalf("got id=%q", id)
	}
	id = matchCommandID(items, "nope")
	if id != "" {
		t.Fatalf("miss should return empty, got id=%q", id)
	}
	// 精确匹配：大小写与空白不做宽容
	id = matchCommandID(items, "Greet")
	if id != "" {
		t.Fatalf("match must be exact, got %q", id)
	}
}

func TestResolveNotFoundErrorShape(t *testing.T) {
	err := commandNotFoundError("nope")
	if err == nil {
		t.Fatalf("err = %v", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeNotFound {
		t.Fatalf("expected api/not_found, got %#v", p)
	}
}
