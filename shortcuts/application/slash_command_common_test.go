// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
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
		if !ok || p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
			t.Errorf("%q: expected validation problem, got %v", bad, err)
		}
	}
}

func TestParseDescriptionI18n_DuplicateLang(t *testing.T) {
	_, err := parseDescriptionI18n([]string{"zh_cn=a", "zh_cn=b"})
	if err == nil {
		t.Fatal("expected duplicate language error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected validation/invalid_argument, got %v", err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Param != "--description-i18n" {
		t.Fatalf("expected param --description-i18n, got %#v", validationErr)
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

func TestIsCommandExists(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "matching code and message",
			err: errs.NewAPIError(errs.SubtypeUnknown,
				"Invalid Param 'command'. command already exists.").WithCode(40000000),
			want: true,
		},
		{
			name: "same message with different code",
			err: errs.NewAPIError(errs.SubtypeUnknown,
				"Invalid Param 'command'. command already exists.").WithCode(40000031),
		},
		{
			name: "same code with different message",
			err: errs.NewAPIError(errs.SubtypeUnknown,
				"Invalid Param 'icon_key'. icon_key is invalid.").WithCode(40000000),
		},
		{name: "nil error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCommandExists(tt.err); got != tt.want {
				t.Fatalf("isCommandExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSlashCommandShortcuts_SharedScopesAcrossIdentities locks in the
// reversal of the OAuth-isolation design: all four slash-command shortcuts
// declare identical scopes for the bot and user identities (plain Scopes /
// ConditionalScopes, no per-identity overrides), so a user-identity
// pre-flight sees the same scope set a bot identity would.
func TestSlashCommandShortcuts_SharedScopesAcrossIdentities(t *testing.T) {
	cases := []struct {
		name            string
		shortcut        common.Shortcut
		wantScope       string
		wantConditional string
		hasConditional  bool
	}{
		{
			name:      "list",
			shortcut:  SlashCommandList,
			wantScope: "application:app_slash_command:read",
		},
		{
			name:            "create",
			shortcut:        SlashCommandCreate,
			wantScope:       "application:app_slash_command:write",
			wantConditional: "application:app_slash_command:read",
			hasConditional:  true,
		},
		{
			name:            "update",
			shortcut:        SlashCommandUpdate,
			wantScope:       "application:app_slash_command:write",
			wantConditional: "application:app_slash_command:read",
			hasConditional:  true,
		},
		{
			name:            "delete",
			shortcut:        SlashCommandDelete,
			wantScope:       "application:app_slash_command:write",
			wantConditional: "application:app_slash_command:read",
			hasConditional:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, identity := range []string{"user", "bot"} {
				declared := tc.shortcut.DeclaredScopesForIdentity(identity)
				if !containsStr(declared, tc.wantScope) {
					t.Errorf("%s: DeclaredScopesForIdentity(%q) = %v, want to contain %q", tc.name, identity, declared, tc.wantScope)
				}
				if tc.hasConditional && !containsStr(declared, tc.wantConditional) {
					t.Errorf("%s: DeclaredScopesForIdentity(%q) = %v, want to contain conditional %q", tc.name, identity, declared, tc.wantConditional)
				}
			}
		})
	}
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
