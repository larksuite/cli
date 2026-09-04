// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/recovery"
)

func TestGetLoginMsg_Zh(t *testing.T) {
	msg := getLoginMsg("zh")
	if msg != loginMsgZh {
		t.Error("expected zh message set")
	}
	if msg.OpenURL != "在浏览器中打开以下链接进行认证:\n\n" {
		t.Errorf("unexpected OpenURL: %s", msg.OpenURL)
	}
	if msg.LoginSuccess != "登录成功! 用户: %s (%s)" {
		t.Errorf("unexpected LoginSuccess: %s", msg.LoginSuccess)
	}
}

func TestGetLoginMsg_En(t *testing.T) {
	msg := getLoginMsg("en")
	if msg != loginMsgEn {
		t.Error("expected en message set")
	}
	if msg.OpenURL != "Open this URL in your browser to authenticate:\n\n" {
		t.Errorf("unexpected OpenURL: %s", msg.OpenURL)
	}
}

func TestGetLoginMsg_DefaultsToZh(t *testing.T) {
	for _, lang := range []i18n.Lang{"", "fr_fr", "ja_jp", "unknown"} {
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
	for _, lang := range []i18n.Lang{i18n.LangZhCN, i18n.LangEnUS} {
		msg := getLoginMsg(lang)

		// LoginSuccess should contain two %s placeholders (userName, openId)
		got := fmt.Sprintf(msg.LoginSuccess, "testuser", "ou_123")
		if got == msg.LoginSuccess {
			t.Errorf("%s LoginSuccess has no format verb", lang)
		}

	}
}

// TestAgentTimeoutHint_CarriesKeyInfo guards the contract that the synchronous
// auth-login output tells AI agents three things: (a) this command blocks for
// minutes — set a long runner timeout, (b) the alternative is the --no-wait +
// --device-code split-flow, and (c) non-streaming harnesses must end the turn
// after presenting the URL instead of blocking in the same turn.
func TestAgentTimeoutHint_CarriesKeyInfo(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.LangZhCN, i18n.LangEnUS} {
		hint := getLoginMsg(lang).AgentTimeoutHint(recovery.RenderContext{})
		for _, want := range []string{"--scope", "--domain", "--recommend", "--exclude", "--no-wait", "--device-code", "turn"} {
			if lang == i18n.LangZhCN && want == "turn" {
				want = "本轮"
			}
			if !strings.Contains(hint, want) {
				t.Errorf("%s AgentTimeoutHint missing %q: %s", lang, want, hint)
			}
		}
		if strings.Contains(hint, "lark-cli auth login --no-wait --json") {
			t.Errorf("%s AgentTimeoutHint recommends an invalid optionless retry: %s", lang, hint)
		}
	}
}

func TestAgentTimeoutHint_DefaultBytesStable(t *testing.T) {
	wantSHA256 := map[i18n.Lang]string{
		i18n.LangZhCN: "9b9d23f6785d7a259de98620184fb05a4952464687a9f60982ce007aee39451e",
		i18n.LangEnUS: "f39c9cd432668401040a4eda43b5ced0d4f20c0b8f55e06ef1773bc4048c6071",
	}
	for lang, want := range wantSHA256 {
		hint := getLoginMsg(lang).AgentTimeoutHint(recovery.RenderContext{})
		if got := fmt.Sprintf("%x", sha256.Sum256([]byte(hint))); got != want {
			t.Errorf("%s default AgentTimeoutHint digest = %s, want legacy %s", lang, got, want)
		}
	}
}

func TestAgentTimeoutHint_ExplicitProfilePreservesStartAndResume(t *testing.T) {
	context := recovery.RenderContext{Profile: "team-beta"}
	for _, lang := range []i18n.Lang{i18n.LangZhCN, i18n.LangEnUS} {
		hint := getLoginMsg(lang).AgentTimeoutHint(context)
		for _, want := range []string{
			"`lark-cli auth login --profile='team-beta'`",
			`"lark-cli auth login --profile='team-beta' --device-code <code>"`,
		} {
			if !strings.Contains(hint, want) {
				t.Errorf("%s profile-aware AgentTimeoutHint missing %q: %s", lang, want, hint)
			}
		}
	}
}
