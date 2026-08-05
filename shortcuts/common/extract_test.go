// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"testing"
)

func TestGetString(t *testing.T) {
	m := map[string]interface{}{
		"name": "Alice",
		"user": map[string]interface{}{
			"id":   "u123",
			"name": "Bob",
			"profile": map[string]interface{}{
				"email": "bob@example.com",
			},
		},
	}

	tests := []struct {
		name string
		keys []string
		want string
	}{
		{"top level", []string{"name"}, "Alice"},
		{"nested one level", []string{"user", "id"}, "u123"},
		{"nested two levels", []string{"user", "profile", "email"}, "bob@example.com"},
		{"missing key", []string{"missing"}, ""},
		{"missing nested", []string{"user", "missing"}, ""},
		{"wrong type", []string{"user"}, ""},
		{"empty keys", []string{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetString(m, tt.keys...)
			if got != tt.want {
				t.Errorf("GetString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetFloat(t *testing.T) {
	m := map[string]interface{}{
		"count": 42.0,
		"data": map[string]interface{}{
			"score": 99.5,
		},
	}

	if got := GetFloat(m, "count"); got != 42.0 {
		t.Errorf("GetFloat(count) = %f, want 42.0", got)
	}
	if got := GetFloat(m, "data", "score"); got != 99.5 {
		t.Errorf("GetFloat(data.score) = %f, want 99.5", got)
	}
	if got := GetFloat(m, "missing"); got != 0 {
		t.Errorf("GetFloat(missing) = %f, want 0", got)
	}
	if got := GetFloat(m); got != 0 {
		t.Errorf("GetFloat() = %f, want 0", got)
	}
}

func TestGetFloatOKDistinguishesMalformedValuesFromZero(t *testing.T) {
	t.Parallel()

	m := map[string]interface{}{
		"zero":   float64(0),
		"null":   nil,
		"string": "0",
	}
	if got, ok := GetFloatOK(m, "zero"); !ok || got != 0 {
		t.Fatalf("GetFloatOK(zero) = (%v, %t), want (0, true)", got, ok)
	}
	for _, key := range []string{"null", "string", "missing"} {
		if got, ok := GetFloatOK(m, key); ok || got != 0 {
			t.Fatalf("GetFloatOK(%s) = (%v, %t), want (0, false)", key, got, ok)
		}
	}
}

func TestGetInt(t *testing.T) {
	m := map[string]interface{}{
		"count":      42,
		"json_count": 7.0,
		"data": map[string]interface{}{
			"score": int64(99),
		},
	}

	if got := GetInt(m, "count"); got != 42 {
		t.Errorf("GetInt(count) = %d, want 42", got)
	}
	if got := GetInt(m, "json_count"); got != 7 {
		t.Errorf("GetInt(json_count) = %d, want 7", got)
	}
	if got := GetInt(m, "data", "score"); got != 99 {
		t.Errorf("GetInt(data.score) = %d, want 99", got)
	}
	if got := GetInt(m, "missing"); got != 0 {
		t.Errorf("GetInt(missing) = %d, want 0", got)
	}
	if got := GetInt(m); got != 0 {
		t.Errorf("GetInt() = %d, want 0", got)
	}
}

func TestGetBool(t *testing.T) {
	m := map[string]interface{}{
		"active": true,
		"data": map[string]interface{}{
			"verified": false,
		},
	}

	if got := GetBool(m, "active"); got != true {
		t.Errorf("GetBool(active) = %v, want true", got)
	}
	if got := GetBool(m, "data", "verified"); got != false {
		t.Errorf("GetBool(data.verified) = %v, want false", got)
	}
	if got := GetBool(m, "missing"); got != false {
		t.Errorf("GetBool(missing) = %v, want false", got)
	}
	if got := GetBool(m); got != false {
		t.Errorf("GetBool() = %v, want false", got)
	}
}

func TestGetMap(t *testing.T) {
	inner := map[string]interface{}{"key": "val"}
	m := map[string]interface{}{
		"data": inner,
	}

	got := GetMap(m, "data")
	if got == nil || got["key"] != "val" {
		t.Errorf("GetMap(data) = %v, want %v", got, inner)
	}
	if got := GetMap(m, "missing"); got != nil {
		t.Errorf("GetMap(missing) = %v, want nil", got)
	}
	// No keys returns the original map.
	if got := GetMap(m); got == nil {
		t.Errorf("GetMap() = nil, want original map")
	}
}

func TestGetSlice(t *testing.T) {
	items := []interface{}{"a", "b"}
	m := map[string]interface{}{
		"items": items,
		"data": map[string]interface{}{
			"list": []interface{}{1.0, 2.0},
		},
	}

	got := GetSlice(m, "items")
	if len(got) != 2 {
		t.Errorf("GetSlice(items) len = %d, want 2", len(got))
	}
	got = GetSlice(m, "data", "list")
	if len(got) != 2 {
		t.Errorf("GetSlice(data.list) len = %d, want 2", len(got))
	}
	if got := GetSlice(m, "missing"); got != nil {
		t.Errorf("GetSlice(missing) = %v, want nil", got)
	}
	if got := GetSlice(m); got != nil {
		t.Errorf("GetSlice() = %v, want nil", got)
	}
}

func TestEachMap(t *testing.T) {
	items := []interface{}{
		map[string]interface{}{"id": "1"},
		"not a map",
		map[string]interface{}{"id": "2"},
		42,
	}

	var ids []string
	EachMap(items, func(m map[string]interface{}) {
		ids = append(ids, m["id"].(string))
	})

	if len(ids) != 2 || ids[0] != "1" || ids[1] != "2" {
		t.Errorf("EachMap collected ids = %v, want [1 2]", ids)
	}
}

func TestNavigateNilMap(t *testing.T) {
	var m map[string]interface{}
	if got := GetString(m, "key"); got != "" {
		t.Errorf("GetString(nil, key) = %q, want empty", got)
	}
}
