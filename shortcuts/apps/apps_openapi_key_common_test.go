// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"reflect"
	"testing"
)

func TestMaskAPIKey(t *testing.T) {
	cases := map[string]string{
		"":                     "****",
		"abcd":                 "****",
		"mdk_live_9f3a2b8c5f4a": "****5f4a",
	}
	for in, want := range cases {
		if got := maskAPIKey(in); got != want {
			t.Errorf("maskAPIKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRedactKeyInfo_StripsRawKey(t *testing.T) {
	in := map[string]interface{}{
		"api_key_id": "1",
		"api_key":    "mdk_live_9f3a2b8c5f4a",
		"name":       "partner-test",
		"status":     float64(1),
	}
	out := redactKeyInfo(in)
	if _, ok := out["api_key"]; ok {
		t.Fatalf("redactKeyInfo must strip api_key, got %v", out)
	}
	if out["key_preview"] != "****5f4a" {
		t.Errorf("key_preview = %v, want ****5f4a", out["key_preview"])
	}
	if out["name"] != "partner-test" || out["api_key_id"] != "1" {
		t.Errorf("non-secret fields must be preserved, got %v", out)
	}
	// input not mutated
	if _, ok := in["api_key"]; !ok {
		t.Errorf("redactKeyInfo must not mutate input")
	}
}

func TestBuildKeyConfig(t *testing.T) {
	// neither provided -> nil
	cfg, err := buildKeyConfig("", false, false)
	if err != nil || cfg != nil {
		t.Fatalf("empty -> nil, got cfg=%v err=%v", cfg, err)
	}
	// scope passthrough + allow-preview
	cfg, err = buildKeyConfig(`[{"method":"GET","path":"/openapi/v1/orders"}]`, true, true)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	wantScope := []interface{}{map[string]interface{}{"method": "GET", "path": "/openapi/v1/orders"}}
	if !reflect.DeepEqual(cfg["request_scope"], wantScope) {
		t.Errorf("request_scope = %v", cfg["request_scope"])
	}
	if cfg["is_allow_access_preview"] != true {
		t.Errorf("is_allow_access_preview = %v", cfg["is_allow_access_preview"])
	}
	// invalid json -> error
	if _, err := buildKeyConfig("{bad", false, false); err == nil {
		t.Errorf("invalid json must error")
	}
}
