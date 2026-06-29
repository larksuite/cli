// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package meta

import (
	"encoding/json"
	"testing"
)

func TestMethod_ParsedAffordance(t *testing.T) {
	// absent / empty / malformed all resolve to ok=false.
	t.Run("nil affordance", func(t *testing.T) {
		if _, ok := (Method{}).ParsedAffordance(); ok {
			t.Error("ParsedAffordance on a method without affordance ok=true, want false")
		}
	})

	notOK := map[string]string{
		"empty payload":        ``,
		"empty object":         `{}`,
		"all empty arrays":     `{"use_when":[],"do_not_use_when":[],"prerequisites":[],"examples":[],"related":[]}`,
		"malformed string":     `"not an object"`,
		"malformed number":     `42`,
		"nested type mismatch": `{"examples":"should be a list"}`,
	}
	for name, raw := range notOK {
		t.Run(name, func(t *testing.T) {
			if _, ok := (Method{Affordance: json.RawMessage(raw)}).ParsedAffordance(); ok {
				t.Errorf("ParsedAffordance(%s) ok=true, want false", raw)
			}
		})
	}

	// Populated affordance parses with all fields.
	raw := `{
		"use_when": ["需要拿到当前用户的主日历 ID"],
		"do_not_use_when": ["已知具体 calendar_id"],
		"prerequisites": ["user 身份登录"],
		"examples": [{"description":"获取主日历","command":"lark-cli calendar calendars primary"}],
		"related": ["calendars.list"]
	}`
	a, ok := (Method{Affordance: json.RawMessage(raw)}).ParsedAffordance()
	if !ok {
		t.Fatal("ParsedAffordance ok=false, want populated")
	}
	if len(a.UseWhen) != 1 || a.UseWhen[0] != "需要拿到当前用户的主日历 ID" {
		t.Errorf("UseWhen = %v", a.UseWhen)
	}
	if len(a.Examples) != 1 || a.Examples[0].Description != "获取主日历" || a.Examples[0].Command != "lark-cli calendar calendars primary" {
		t.Errorf("Examples = %+v", a.Examples)
	}
	if len(a.Related) != 1 || a.Related[0] != "calendars.list" {
		t.Errorf("Related = %v", a.Related)
	}
}
