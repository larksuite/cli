// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"reflect"
	"testing"
)

func TestMaskAPIKey(t *testing.T) {
	cases := map[string]string{
		"":                      "****",
		"abcd":                  "****",
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

func TestParseScopeAPI(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		info, err := parseScopeAPI("GET /openapi/v1/orders")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info["httpMethod"] != "GET" {
			t.Errorf("httpMethod = %v, want GET", info["httpMethod"])
		}
		if info["httpPath"] != "/openapi/v1/orders" {
			t.Errorf("httpPath = %v, want /openapi/v1/orders", info["httpPath"])
		}
	})
	t.Run("lowercase method uppercased", func(t *testing.T) {
		info, err := parseScopeAPI("post /openapi/x")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info["httpMethod"] != "POST" {
			t.Errorf("httpMethod = %v, want POST", info["httpMethod"])
		}
	})
	t.Run("too few fields", func(t *testing.T) {
		if _, err := parseScopeAPI("GET"); err == nil {
			t.Errorf("one-word input must error")
		}
	})
	t.Run("too many fields", func(t *testing.T) {
		if _, err := parseScopeAPI("GET /openapi/x extra"); err == nil {
			t.Errorf("three-word input must error")
		}
	})
}

func TestBuildRequestScope(t *testing.T) {
	t.Run("nothing set -> nil", func(t *testing.T) {
		rs, err := buildRequestScope(false, nil, "")
		if err != nil || rs != nil {
			t.Fatalf("expected nil,nil got rs=%v err=%v", rs, err)
		}
	})
	t.Run("scope-all only", func(t *testing.T) {
		rs, err := buildRequestScope(true, nil, "")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		m := rs.(map[string]interface{})
		if m["allowAll"] != true {
			t.Errorf("allowAll = %v, want true", m["allowAll"])
		}
		if _, ok := m["httpInfos"]; ok {
			t.Errorf("httpInfos should not appear when no scope-api provided")
		}
	})
	t.Run("scope-api adds httpInfos", func(t *testing.T) {
		rs, err := buildRequestScope(false, []string{"GET /openapi/x"}, "")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		m := rs.(map[string]interface{})
		if m["allowAll"] != false {
			t.Errorf("allowAll = %v, want false", m["allowAll"])
		}
		infos := m["httpInfos"].([]interface{})
		if len(infos) != 1 {
			t.Fatalf("httpInfos len = %d, want 1", len(infos))
		}
		info := infos[0].(map[string]interface{})
		if info["httpMethod"] != "GET" || info["httpPath"] != "/openapi/x" {
			t.Errorf("info = %v", info)
		}
	})
	t.Run("raw scope passthrough", func(t *testing.T) {
		rs, err := buildRequestScope(false, nil, `{"allowAll":true}`)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		m := rs.(map[string]interface{})
		if m["allowAll"] != true {
			t.Errorf("allowAll = %v, want true", m["allowAll"])
		}
	})
	t.Run("raw + scope-all -> error", func(t *testing.T) {
		if _, err := buildRequestScope(true, nil, `{"allowAll":true}`); err == nil {
			t.Errorf("raw + scope-all must error")
		}
	})
	t.Run("raw + scope-api -> error", func(t *testing.T) {
		if _, err := buildRequestScope(false, []string{"GET /openapi/x"}, `{"allowAll":true}`); err == nil {
			t.Errorf("raw + scope-api must error")
		}
	})
	t.Run("invalid raw json -> error", func(t *testing.T) {
		if _, err := buildRequestScope(false, nil, "{bad"); err == nil {
			t.Errorf("invalid json must error")
		}
	})
}

func TestBuildKeyConfig(t *testing.T) {
	t.Run("nothing set -> nil", func(t *testing.T) {
		cfg, err := buildKeyConfig(false, nil, "", false, false)
		if err != nil || cfg != nil {
			t.Fatalf("empty -> nil, got cfg=%v err=%v", cfg, err)
		}
	})
	t.Run("scope-all -> camelCase requestScope", func(t *testing.T) {
		cfg, err := buildKeyConfig(true, nil, "", false, false)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		rs := cfg["requestScope"].(map[string]interface{})
		if rs["allowAll"] != true {
			t.Errorf("allowAll = %v, want true", rs["allowAll"])
		}
		if _, ok := cfg["isAllowAccessPreview"]; ok {
			t.Errorf("isAllowAccessPreview should not appear")
		}
	})
	t.Run("scope-api -> camelCase httpInfos", func(t *testing.T) {
		cfg, err := buildKeyConfig(false, []string{"GET /openapi/x"}, "", false, false)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		rs := cfg["requestScope"].(map[string]interface{})
		if rs["allowAll"] != false {
			t.Errorf("allowAll = %v, want false", rs["allowAll"])
		}
		infos := rs["httpInfos"].([]interface{})
		if len(infos) != 1 {
			t.Fatalf("httpInfos len = %d, want 1", len(infos))
		}
		info := infos[0].(map[string]interface{})
		if info["httpMethod"] != "GET" || info["httpPath"] != "/openapi/x" {
			t.Errorf("info = %v", info)
		}
	})
	t.Run("raw scope passthrough", func(t *testing.T) {
		cfg, err := buildKeyConfig(false, nil, `{"allowAll":true}`, false, false)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		rs := cfg["requestScope"].(map[string]interface{})
		if rs["allowAll"] != true {
			t.Errorf("allowAll = %v", rs["allowAll"])
		}
	})
	t.Run("allow-preview only -> isAllowAccessPreview", func(t *testing.T) {
		cfg, err := buildKeyConfig(false, nil, "", true, true)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if _, ok := cfg["requestScope"]; ok {
			t.Errorf("requestScope should not appear when not set")
		}
		if cfg["isAllowAccessPreview"] != true {
			t.Errorf("isAllowAccessPreview = %v, want true", cfg["isAllowAccessPreview"])
		}
	})
	t.Run("scope-all + allow-preview -> both camelCase keys", func(t *testing.T) {
		cfg, err := buildKeyConfig(true, nil, "", true, false)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if _, ok := cfg["requestScope"]; !ok {
			t.Errorf("requestScope missing")
		}
		if cfg["isAllowAccessPreview"] != false {
			t.Errorf("isAllowAccessPreview = %v, want false", cfg["isAllowAccessPreview"])
		}
		// ensure no snake_case keys
		if _, ok := cfg["request_scope"]; ok {
			t.Errorf("found snake_case key request_scope — must use camelCase")
		}
		if _, ok := cfg["is_allow_access_preview"]; ok {
			t.Errorf("found snake_case key is_allow_access_preview — must use camelCase")
		}
	})
	t.Run("raw + scope-all -> error", func(t *testing.T) {
		if _, err := buildKeyConfig(true, nil, `{"allowAll":true}`, false, false); err == nil {
			t.Errorf("raw + scope-all must error")
		}
	})
	t.Run("invalid json -> error", func(t *testing.T) {
		if _, err := buildKeyConfig(false, nil, "{bad", false, false); err == nil {
			t.Errorf("invalid json must error")
		}
	})
	t.Run("no snake_case keys emitted", func(t *testing.T) {
		cfg, err := buildKeyConfig(false, []string{"GET /openapi/x"}, "", true, true)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if _, ok := cfg["request_scope"]; ok {
			t.Errorf("snake_case request_scope must not appear")
		}
		if _, ok := cfg["is_allow_access_preview"]; ok {
			t.Errorf("snake_case is_allow_access_preview must not appear")
		}
		rs := cfg["requestScope"].(map[string]interface{})
		infos := rs["httpInfos"].([]interface{})
		info := infos[0].(map[string]interface{})
		wantInfo := map[string]interface{}{"httpMethod": "GET", "httpPath": "/openapi/x"}
		if !reflect.DeepEqual(info, wantInfo) {
			t.Errorf("info = %v, want %v", info, wantInfo)
		}
	})
}
