// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"reflect"
	"testing"
)

func TestMaskAPIKey(t *testing.T) {
	cases := map[string]string{
		"":             "****",
		"abcd":         "****",
		"xxxxxxxxxxxx": "****xxxx",
	}
	for in, want := range cases {
		if got := maskAPIKey(in); got != want {
			t.Errorf("maskAPIKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRedactKeyInfo_StripsRawKey(t *testing.T) {
	in := map[string]interface{}{
		"api_key_id": "k1",
		"api_key":    "xxxxxxxxxxxx",
		"name":       "partner-test",
		"status":     float64(1),
	}
	out := redactKeyInfo(in)
	if _, ok := out["api_key"]; ok {
		t.Fatalf("redactKeyInfo must strip api_key, got %v", out)
	}
	if out["key_preview"] != "****xxxx" {
		t.Errorf("key_preview = %v, want ****xxxx", out["key_preview"])
	}
	if out["name"] != "partner-test" || out["api_key_id"] != "k1" {
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
		if info["http_method"] != "GET" {
			t.Errorf("http_method = %v, want GET", info["http_method"])
		}
		if info["http_path"] != "/openapi/v1/orders" {
			t.Errorf("http_path = %v, want /openapi/v1/orders", info["http_path"])
		}
	})
	t.Run("lowercase method uppercased", func(t *testing.T) {
		info, err := parseScopeAPI("post /openapi/x")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info["http_method"] != "POST" {
			t.Errorf("http_method = %v, want POST", info["http_method"])
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

func TestValidateScopeAPIMethod(t *testing.T) {
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		if err := validateScopeAPIMethod(m); err != nil {
			t.Errorf("validateScopeAPIMethod(%q) = %v, want nil", m, err)
		}
	}
	for _, m := range []string{"TRACE", "CONNECT", "OPTIONS", "HEAD", "", "get"} {
		if err := validateScopeAPIMethod(m); err == nil {
			t.Errorf("validateScopeAPIMethod(%q) = nil, want error", m)
		}
	}
}

func TestValidateScopeAPIPath(t *testing.T) {
	for _, p := range []string{"/openapi/orders", "/openapi/v1/x"} {
		if err := validateScopeAPIPath(p); err != nil {
			t.Errorf("validateScopeAPIPath(%q) = %v, want nil", p, err)
		}
	}
	for _, p := range []string{"", "openapi/x", "/openapi/../admin", "/..", "/openapi//x", "//x"} {
		if err := validateScopeAPIPath(p); err == nil {
			t.Errorf("validateScopeAPIPath(%q) = nil, want error", p)
		}
	}
}

func TestValidateRequestScopeFields(t *testing.T) {
	ok := []map[string]interface{}{
		{"allow_all": true},
		{"allow_all": false, "http_infos": []interface{}{
			map[string]interface{}{"http_method": "GET", "http_path": "/openapi/x"},
		}},
		{},
	}
	for _, rs := range ok {
		if err := validateRequestScopeFields(rs); err != nil {
			t.Errorf("validateRequestScopeFields(%v) = %v, want nil", rs, err)
		}
	}
	bad := []map[string]interface{}{
		{"foo": 1},                         // unknown top-level field
		{"allow_all": "yes"},               // wrong type
		{"http_infos": "x"},                // not an array
		{"http_infos": []interface{}{"x"}}, // entry not an object
		{"http_infos": []interface{}{map[string]interface{}{"http_method": "TRACE", "http_path": "/x"}}},           // bad method
		{"http_infos": []interface{}{map[string]interface{}{"http_method": "GET", "http_path": "../x"}}},           // bad path
		{"http_infos": []interface{}{map[string]interface{}{"http_method": "GET", "http_path": "/x", "extra": 1}}}, // unknown entry field
	}
	for _, rs := range bad {
		if err := validateRequestScopeFields(rs); err == nil {
			t.Errorf("validateRequestScopeFields(%v) = nil, want error", rs)
		}
	}
}

func TestParseRawScope(t *testing.T) {
	if _, err := parseRawScope(`{"allow_all":true}`); err != nil {
		t.Errorf("valid object errored: %v", err)
	}
	for _, raw := range []string{`["x"]`, `"s"`, `123`, `{"foo":1}`, `{bad`} {
		if _, err := parseRawScope(raw); err == nil {
			t.Errorf("parseRawScope(%q) = nil, want error", raw)
		}
	}
}

func TestParseScopeAPI_Rejects(t *testing.T) {
	bad := []string{"TRACE /openapi/x", "CONNECT /x", "GET ../admin", "GET openapi/x", "GET /a//b"}
	for _, in := range bad {
		if _, err := parseScopeAPI(in); err == nil {
			t.Errorf("parseScopeAPI(%q) = nil, want error", in)
		}
	}
	// regression: legitimate input still parses (and lowercases the method)
	info, err := parseScopeAPI("get /openapi/orders")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info["http_method"] != "GET" || info["http_path"] != "/openapi/orders" {
		t.Errorf("info = %v", info)
	}
}

func TestBuildRequestScope_RawValidation(t *testing.T) {
	// unknown field now rejected (HIGH-2)
	if _, err := buildRequestScope(false, nil, `{"foo":1}`); err == nil {
		t.Errorf("raw scope with unknown field must error")
	}
	// non-object rejected
	if _, err := buildRequestScope(false, nil, `["x"]`); err == nil {
		t.Errorf("non-object raw scope must error")
	}
	// nested bad method rejected
	if _, err := buildRequestScope(false, nil, `{"http_infos":[{"http_method":"TRACE","http_path":"/x"}]}`); err == nil {
		t.Errorf("raw scope with bad nested method must error")
	}
	// regression: documented fields pass
	if _, err := buildRequestScope(false, nil, `{"allow_all":true}`); err != nil {
		t.Errorf("valid raw scope errored: %v", err)
	}
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
		if m["allow_all"] != true {
			t.Errorf("allow_all = %v, want true", m["allow_all"])
		}
		if _, ok := m["http_infos"]; ok {
			t.Errorf("http_infos should not appear when no scope-api provided")
		}
	})
	t.Run("scope-api adds http_infos", func(t *testing.T) {
		rs, err := buildRequestScope(false, []string{"GET /openapi/x"}, "")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		m := rs.(map[string]interface{})
		if m["allow_all"] != false {
			t.Errorf("allow_all = %v, want false", m["allow_all"])
		}
		infos := m["http_infos"].([]interface{})
		if len(infos) != 1 {
			t.Fatalf("http_infos len = %d, want 1", len(infos))
		}
		info := infos[0].(map[string]interface{})
		if info["http_method"] != "GET" || info["http_path"] != "/openapi/x" {
			t.Errorf("info = %v", info)
		}
	})
	t.Run("raw scope passthrough", func(t *testing.T) {
		rs, err := buildRequestScope(false, nil, `{"allow_all":true}`)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		m := rs.(map[string]interface{})
		if m["allow_all"] != true {
			t.Errorf("allow_all = %v, want true", m["allow_all"])
		}
	})
	t.Run("raw + scope-all -> error", func(t *testing.T) {
		if _, err := buildRequestScope(true, nil, `{"allow_all":true}`); err == nil {
			t.Errorf("raw + scope-all must error")
		}
	})
	t.Run("raw + scope-api -> error", func(t *testing.T) {
		if _, err := buildRequestScope(false, []string{"GET /openapi/x"}, `{"allow_all":true}`); err == nil {
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
	t.Run("scope-all -> snake_case request_scope", func(t *testing.T) {
		cfg, err := buildKeyConfig(true, nil, "", false, false)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		rs := cfg["request_scope"].(map[string]interface{})
		if rs["allow_all"] != true {
			t.Errorf("allow_all = %v, want true", rs["allow_all"])
		}
		if _, ok := cfg["is_allow_access_preview"]; ok {
			t.Errorf("is_allow_access_preview should not appear")
		}
	})
	t.Run("scope-api -> snake_case http_infos", func(t *testing.T) {
		cfg, err := buildKeyConfig(false, []string{"GET /openapi/x"}, "", false, false)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		rs := cfg["request_scope"].(map[string]interface{})
		if rs["allow_all"] != false {
			t.Errorf("allow_all = %v, want false", rs["allow_all"])
		}
		infos := rs["http_infos"].([]interface{})
		if len(infos) != 1 {
			t.Fatalf("http_infos len = %d, want 1", len(infos))
		}
		info := infos[0].(map[string]interface{})
		if info["http_method"] != "GET" || info["http_path"] != "/openapi/x" {
			t.Errorf("info = %v", info)
		}
	})
	t.Run("raw scope passthrough", func(t *testing.T) {
		cfg, err := buildKeyConfig(false, nil, `{"allow_all":true}`, false, false)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		rs := cfg["request_scope"].(map[string]interface{})
		if rs["allow_all"] != true {
			t.Errorf("allow_all = %v", rs["allow_all"])
		}
	})
	t.Run("allow-preview only -> is_allow_access_preview", func(t *testing.T) {
		cfg, err := buildKeyConfig(false, nil, "", true, true)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if _, ok := cfg["request_scope"]; ok {
			t.Errorf("request_scope should not appear when not set")
		}
		if cfg["is_allow_access_preview"] != true {
			t.Errorf("is_allow_access_preview = %v, want true", cfg["is_allow_access_preview"])
		}
	})
	t.Run("scope-all + allow-preview -> both snake_case keys", func(t *testing.T) {
		cfg, err := buildKeyConfig(true, nil, "", true, false)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if _, ok := cfg["request_scope"]; !ok {
			t.Errorf("request_scope missing")
		}
		if cfg["is_allow_access_preview"] != false {
			t.Errorf("is_allow_access_preview = %v, want false", cfg["is_allow_access_preview"])
		}
		// ensure no camelCase keys
		if _, ok := cfg["requestScope"]; ok {
			t.Errorf("found camelCase key requestScope — must use snake_case")
		}
		if _, ok := cfg["isAllowAccessPreview"]; ok {
			t.Errorf("found camelCase key isAllowAccessPreview — must use snake_case")
		}
	})
	t.Run("raw + scope-all -> error", func(t *testing.T) {
		if _, err := buildKeyConfig(true, nil, `{"allow_all":true}`, false, false); err == nil {
			t.Errorf("raw + scope-all must error")
		}
	})
	t.Run("invalid json -> error", func(t *testing.T) {
		if _, err := buildKeyConfig(false, nil, "{bad", false, false); err == nil {
			t.Errorf("invalid json must error")
		}
	})
	t.Run("no camelCase keys emitted", func(t *testing.T) {
		cfg, err := buildKeyConfig(false, []string{"GET /openapi/x"}, "", true, true)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if _, ok := cfg["requestScope"]; ok {
			t.Errorf("camelCase requestScope must not appear")
		}
		if _, ok := cfg["isAllowAccessPreview"]; ok {
			t.Errorf("camelCase isAllowAccessPreview must not appear")
		}
		rs := cfg["request_scope"].(map[string]interface{})
		infos := rs["http_infos"].([]interface{})
		info := infos[0].(map[string]interface{})
		wantInfo := map[string]interface{}{"http_method": "GET", "http_path": "/openapi/x"}
		if !reflect.DeepEqual(info, wantInfo) {
			t.Errorf("info = %v, want %v", info, wantInfo)
		}
	})
}
