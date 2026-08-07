// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/brand"
	"github.com/larksuite/cli/internal/identity"
	"github.com/larksuite/cli/internal/secret"
)

func TestMultiAppConfig_StrictMode_JSON(t *testing.T) {
	// StrictMode="" should be omitted (omitempty)
	m := &MultiAppConfig{
		Apps: []AppConfig{{AppId: "a", AppSecret: secret.PlainSecret("s"), Brand: brand.Feishu, Users: []AppUser{}}},
	}
	data, _ := json.Marshal(m)
	if string(data) != `{"apps":[{"appId":"a","appSecret":"s","brand":"feishu","users":[]}]}` {
		t.Errorf("StrictMode empty should be omitted, got: %s", data)
	}

	// StrictMode="bot" should be present
	m.StrictMode = identity.StrictModeBot
	data, _ = json.Marshal(m)
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)
	if parsed["strictMode"] != "bot" {
		t.Errorf("StrictMode=bot should be present, got: %s", data)
	}
}

func TestAppConfig_StrictMode_JSON(t *testing.T) {
	// StrictMode nil should be omitted
	app := &AppConfig{AppId: "a", AppSecret: secret.PlainSecret("s"), Brand: brand.Feishu, Users: []AppUser{}}
	data, _ := json.Marshal(app)
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)
	if _, ok := parsed["strictMode"]; ok {
		t.Errorf("nil StrictMode should be omitted, got: %s", data)
	}

	// StrictMode = pointer to "user"
	v := identity.StrictModeUser
	app.StrictMode = &v
	data, _ = json.Marshal(app)
	json.Unmarshal(data, &parsed)
	if parsed["strictMode"] != "user" {
		t.Errorf("StrictMode=user should be present, got: %s", data)
	}

	// StrictMode = pointer to "off" (explicit off — should be present, not omitted)
	voff := identity.StrictModeOff
	app.StrictMode = &voff
	data, _ = json.Marshal(app)
	json.Unmarshal(data, &parsed)
	if val, ok := parsed["strictMode"]; !ok || val != "off" {
		t.Errorf("StrictMode=off (explicit) should be present, got: %s", data)
	}
}
