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
	for _, lang := range []i18n.Lang{"", "unknown", "xyz", "invalid"} {
		msg := getInitMsg(lang)
		if msg != initMsgZh {
			t.Errorf("getInitMsg(%q) should default to zh", lang)
		}
	}
}

func TestGetInitMsg_Vi(t *testing.T) {
	msg := getInitMsg(i18n.LangViVN)
	if msg != initMsgVi {
		t.Error("expected vi message set")
	}
	if msg.SelectAction != "Chọn thao tác" {
		t.Errorf("unexpected SelectAction: %s", msg.SelectAction)
	}
}

func TestInitMsgZh_AllFieldsNonEmpty(t *testing.T) {
	assertAllFieldsNonEmpty(t, initMsgZh, "zh")
}

func TestInitMsgEn_AllFieldsNonEmpty(t *testing.T) {
	assertAllFieldsNonEmpty(t, initMsgEn, "en")
}

func TestInitMsgVi_AllFieldsNonEmpty(t *testing.T) {
	assertAllFieldsNonEmpty(t, initMsgVi, "vi")
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
		"LangPreferenceSet":    msg.LangPreferenceSet,
	}
	for name, val := range fields {
		if val == "" {
			t.Errorf("%s.%s is empty", label, name)
		}
	}
}

func TestInitMsg_FormatStrings(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.LangZhCN, i18n.LangEnUS, i18n.LangViVN} {
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

func TestGetInitMsg_BilingualCollapse(t *testing.T) {
	tests := []struct {
		lang   i18n.Lang
		wantZh bool
		wantEn bool
		wantVi bool
	}{
		{i18n.LangZhCN, true, false, false},
		{i18n.LangEnUS, false, true, false},
		{"en", false, true, false},
		{i18n.LangJaJP, true, false, false},
		{i18n.LangViVN, false, false, true},
		{"fr_fr", true, false, false},
		{"invalid", true, false, false},
		{"", true, false, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			msg := getInitMsg(tt.lang)
			if msg == nil {
				t.Fatal("getInitMsg returned nil")
			}
			var want *initMsg
			switch {
			case tt.wantEn:
				want = initMsgEn
			case tt.wantVi:
				want = initMsgVi
			default:
				want = initMsgZh
			}
			if msg != want {
				t.Errorf("getInitMsg(%q) returned wrong struct", tt.lang)
			}
		})
	}
}
