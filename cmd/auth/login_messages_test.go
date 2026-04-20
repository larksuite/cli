// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"
	"reflect"
	"testing"
)

func TestGetLoginMsg_Zh(t *testing.T) {
	msg := getLoginMsg("zh")
	if msg != loginMsgZh {
		t.Error("expected zh message set")
	}
	if msg.SelectDomains != "选择要授权的业务域" {
		t.Errorf("unexpected SelectDomains: %s", msg.SelectDomains)
	}
}

func TestGetLoginMsg_En(t *testing.T) {
	msg := getLoginMsg("en")
	if msg != loginMsgEn {
		t.Error("expected en message set")
	}
	if msg.SelectDomains != "Select domains to authorize" {
		t.Errorf("unexpected SelectDomains: %s", msg.SelectDomains)
	}
}

func TestGetLoginMsg_DefaultsToZh(t *testing.T) {
	for _, lang := range []string{"", "fr", "ja", "unknown"} {
		msg := getLoginMsg(lang)
		if msg != loginMsgZh {
			t.Errorf("getLoginMsg(%q) should default to zh", lang)
		}
	}
}

func TestLoginMsgZh_AllFieldsNonEmpty(t *testing.T) {
	assertLoginMsgAllFieldsNonEmpty(t, loginMsgZh, "zh")
}

func TestLoginMsgEn_AllFieldsNonEmpty(t *testing.T) {
	assertLoginMsgAllFieldsNonEmpty(t, loginMsgEn, "en")
}

func assertLoginMsgAllFieldsNonEmpty(t *testing.T, msg *loginMsg, label string) {
	t.Helper()
	v := reflect.ValueOf(*msg)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := typ.Field(i)
		val := v.Field(i).String()
		if val == "" {
			t.Errorf("%s.%s is empty", label, field.Name)
		}
	}
}

func TestLoginMsg_FormatStrings(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		msg := getLoginMsg(lang)

		// LoginSuccess should contain two %s placeholders (userName, openId)
		got := fmt.Sprintf(msg.LoginSuccess, "testuser", "ou_123")
		if got == msg.LoginSuccess {
			t.Errorf("%s LoginSuccess has no format verb", lang)
		}

		// AuthorizedUser should contain two %s placeholders (userName, openId)
		got = fmt.Sprintf(msg.AuthorizedUser, "testuser", "ou_123")
		if got == msg.AuthorizedUser {
			t.Errorf("%s AuthorizedUser has no format verb", lang)
		}

		// SummaryDomains should contain %s
		got = fmt.Sprintf(msg.SummaryDomains, "calendar, task")
		if got == msg.SummaryDomains {
			t.Errorf("%s SummaryDomains has no format verb", lang)
		}

		// SummaryPerm should contain %s
		got = fmt.Sprintf(msg.SummaryPerm, "all")
		if got == msg.SummaryPerm {
			t.Errorf("%s SummaryPerm has no format verb", lang)
		}

		// SummaryScopes should contain %d and %s
		got = fmt.Sprintf(msg.SummaryScopes, 5, "a, b, c")
		if got == msg.SummaryScopes {
			t.Errorf("%s SummaryScopes has no format verb", lang)
		}
	}
}
