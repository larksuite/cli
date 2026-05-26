// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"testing"

	"github.com/larksuite/cli/internal/i18n"
)

func TestGetInitMsg_Zh(t *testing.T) {
	msg := getInitMsg("zh")
	if msg != initMsgZh {
		t.Error("expected zh message set")
	}
	if msg.SelectAction != "选择操作" {
		t.Errorf("unexpected SelectAction: %s", msg.SelectAction)
	}
}

func TestGetInitMsg_En(t *testing.T) {
	msg := getInitMsg("en")
	if msg != initMsgEn {
		t.Error("expected en message set")
	}
	if msg.SelectAction != "Select action" {
		t.Errorf("unexpected SelectAction: %s", msg.SelectAction)
	}
}

func TestGetInitMsg_DefaultsToZh(t *testing.T) {
	for _, lang := range []string{"", "unknown", "xyz", "invalid"} {
		msg := getInitMsg(lang)
		if msg != initMsgZh {
			t.Errorf("getInitMsg(%q) should default to zh", lang)
		}
	}
}

func TestInitMsgZh_AllFieldsNonEmpty(t *testing.T) {
	assertAllFieldsNonEmpty(t, initMsgZh, "zh")
}

func TestInitMsgEn_AllFieldsNonEmpty(t *testing.T) {
	assertAllFieldsNonEmpty(t, initMsgEn, "en")
}

func assertAllFieldsNonEmpty(t *testing.T, msg *initMsg, label string) {
	t.Helper()
	fields := map[string]string{
		"SelectAction":         msg.SelectAction,
		"CreateNewApp":         msg.CreateNewApp,
		"ConfigExistingApp":    msg.ConfigExistingApp,
		"Platform":             msg.Platform,
		"SelectPlatform":       msg.SelectPlatform,
		"Feishu":               msg.Feishu,
		"ScanQRCode":           msg.ScanQRCode,
		"ScanOrOpenLink":       msg.ScanOrOpenLink,
		"WaitingForScan":       msg.WaitingForScan,
		"OpenLinkNonTTY":       msg.OpenLinkNonTTY,
		"WaitingForScanNonTTY": msg.WaitingForScanNonTTY,
		"DetectedLarkTenant":   msg.DetectedLarkTenant,
		"AppCreated":           msg.AppCreated,
		"ConfigSaved":          msg.ConfigSaved,
	}
	for name, val := range fields {
		if val == "" {
			t.Errorf("%s.%s is empty", label, name)
		}
	}
}

func TestInitMsg_FormatStrings(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		msg := getInitMsg(lang)
		// AppCreated and ConfigSaved should contain %s for App ID
		got := fmt.Sprintf(msg.AppCreated, "cli_test123")
		if got == msg.AppCreated {
			t.Errorf("%s AppCreated has no format verb", lang)
		}
		got = fmt.Sprintf(msg.ConfigSaved, "cli_test123")
		if got == msg.ConfigSaved {
			t.Errorf("%s ConfigSaved has no format verb", lang)
		}
	}
}

func TestGetInitMsg_Multilang(t *testing.T) {
	tests := []struct {
		lang         string
		shouldBeZh   bool
		shouldBeEn   bool
		shouldBeLang string // specific lang to check non-nil
	}{
		{"zh", true, false, ""},
		{"en", false, true, ""},
		{"ja", false, false, "ja"},
		{"ko", false, false, "ko"},
		{"invalid", true, false, ""}, // fallback to zh
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			msg := getInitMsg(tt.lang)
			if msg == nil {
				t.Fatal("getInitMsg returned nil")
			}

			// Check fallback behavior
			if tt.shouldBeZh && msg.SelectAction != initMsgZh.SelectAction {
				t.Error("Expected Chinese message for fallback")
			}
			if tt.shouldBeEn && msg.SelectAction != initMsgEn.SelectAction {
				t.Error("Expected English message")
			}
		})
	}
}

func TestPromptLangSelection_Validation(t *testing.T) {
	// Test that all valid languages are accepted
	for _, lang := range i18n.ValidLanguages {
		if !i18n.IsValidLang(lang) {
			t.Errorf("Language %s should be valid", lang)
		}
	}
}
