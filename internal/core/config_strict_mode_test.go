// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"encoding/json"
	"testing"
)

func TestMultiAppConfig_StrictMode_JSON(t *testing.T) {
	// StrictMode="" should be omitted (omitempty)
	m := &MultiAppConfig{
		Apps: []AppConfig{{AppId: "a", AppSecret: PlainSecret("s"), Brand: BrandFeishu, Users: []AppUser{}}},
	}
	data, _ := json.Marshal(m)
	if string(data) != `{"apps":[{"appId":"a","appSecret":"s","brand":"feishu","users":[]}]}` {
		t.Errorf("StrictMode empty should be omitted, got: %s", data)
	}

	// StrictMode="bot" should be present
	m.StrictMode = StrictModeBot
	data, _ = json.Marshal(m)
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)
	if parsed["strictMode"] != "bot" {
		t.Errorf("StrictMode=bot should be present, got: %s", data)
	}
}

func TestAppConfig_StrictMode_JSON(t *testing.T) {
	// StrictMode nil should be omitted
	app := &AppConfig{AppId: "a", AppSecret: PlainSecret("s"), Brand: BrandFeishu, Users: []AppUser{}}
	data, _ := json.Marshal(app)
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)
	if _, ok := parsed["strictMode"]; ok {
		t.Errorf("nil StrictMode should be omitted, got: %s", data)
	}

	// StrictMode = pointer to "user"
	v := StrictModeUser
	app.StrictMode = &v
	data, _ = json.Marshal(app)
	json.Unmarshal(data, &parsed)
	if parsed["strictMode"] != "user" {
		t.Errorf("StrictMode=user should be present, got: %s", data)
	}

	// StrictMode = pointer to "off" (explicit off — should be present, not omitted)
	voff := StrictModeOff
	app.StrictMode = &voff
	data, _ = json.Marshal(app)
	json.Unmarshal(data, &parsed)
	if val, ok := parsed["strictMode"]; !ok || val != "off" {
		t.Errorf("StrictMode=off (explicit) should be present, got: %s", data)
	}
}
