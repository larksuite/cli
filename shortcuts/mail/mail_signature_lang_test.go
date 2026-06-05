// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestResolveLang(t *testing.T) {
	tests := []struct {
		name   string
		stored i18n.Lang
		want   string
	}{
		{"english", i18n.LangEnUS, "en_us"},
		{"japanese", i18n.LangJaJP, "ja_jp"},
		{"chinese", i18n.LangZhCN, "zh_cn"},
		{"legacy short en", "en", "en_us"},
		{"unsupported-by-mail falls back to zh_cn", i18n.LangFrFR, "zh_cn"},
		{"unset falls back to zh_cn", "", "zh_cn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &common.RuntimeContext{Config: &core.CliConfig{Lang: tt.stored}}
			if got := resolveLang(rt); got != tt.want {
				t.Errorf("resolveLang(stored=%q) = %q, want %q", tt.stored, got, tt.want)
			}
		})
	}
}
