// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestParseDescriptionI18n_OK(t *testing.T) {
	m, err := parseDescriptionI18n([]string{"zh_cn=你好", "en_us=Hello=World"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["zh_cn"] != "你好" {
		t.Errorf("zh_cn = %q", m["zh_cn"])
	}
	// 只按首个 = 分割：值内可含 =
	if m["en_us"] != "Hello=World" {
		t.Errorf("en_us = %q", m["en_us"])
	}
}

func TestParseDescriptionI18n_Empty(t *testing.T) {
	m, err := parseDescriptionI18n(nil)
	if err != nil || m != nil {
		t.Fatalf("nil input: m=%v err=%v", m, err)
	}
}

func TestParseDescriptionI18n_BadFormat(t *testing.T) {
	for _, bad := range []string{"zh_cn", "=text", "zh_cn=", "  =x"} {
		_, err := parseDescriptionI18n([]string{bad})
		if err == nil {
			t.Errorf("%q: expected error", bad)
			continue
		}
		p, ok := errs.ProblemOf(err)
		if !ok || p.Category != errs.CategoryValidation {
			t.Errorf("%q: expected validation problem, got %v", bad, err)
		}
	}
}

func TestParseDescriptionI18n_DuplicateLang(t *testing.T) {
	_, err := parseDescriptionI18n([]string{"zh_cn=a", "zh_cn=b"})
	if err == nil || !strings.Contains(err.Error(), "duplicate language") {
		t.Fatalf("expected duplicate language error, got %v", err)
	}
}

func TestValidateCommandName(t *testing.T) {
	if err := validateCommandName("greet", "--command"); err != nil {
		t.Fatalf("greet: %v", err)
	}
	for _, bad := range []string{"", "  ", "/greet"} {
		if err := validateCommandName(bad, "--command"); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestBuildSlashCommandBody(t *testing.T) {
	body := buildSlashCommandBody("greet", "hi", map[string]string{"zh_cn": "你好"}, "skill_outlined")
	if body["command"] != "greet" {
		t.Errorf("command = %v", body["command"])
	}
	desc := body["description"].(map[string]interface{})
	if desc["default_value"] != "hi" {
		t.Errorf("default_value = %v", desc["default_value"])
	}
	if desc["i18n"].(map[string]string)["zh_cn"] != "你好" {
		t.Errorf("i18n = %v", desc["i18n"])
	}
	// icon 与 description 顶层平级（实测钉死，文档 create 示例是笔误）
	if body["icon"].(map[string]interface{})["icon_key"] != "skill_outlined" {
		t.Errorf("icon = %v", body["icon"])
	}
	// partial：不提供的字段不出现（PATCH 语义依赖）
	partial := buildSlashCommandBody("", "", nil, "skill_outlined")
	if _, has := partial["command"]; has {
		t.Error("empty command must be omitted")
	}
	if _, has := partial["description"]; has {
		t.Error("empty description must be omitted")
	}
}
