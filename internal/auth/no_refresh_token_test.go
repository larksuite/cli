// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

func TestNoRefreshTokenSettingsURL(t *testing.T) {
	if got := NoRefreshTokenSettingsURL(core.BrandFeishu, "cli_abc"); got != "https://open.feishu.cn/app/cli_abc/safe" {
		t.Fatalf("feishu URL = %q", got)
	}
	if got := NoRefreshTokenSettingsURL(core.BrandLark, "cli_abc"); !strings.HasSuffix(got, "/app/cli_abc/safe") || strings.Contains(got, "feishu") {
		t.Fatalf("lark URL = %q", got)
	}
	if got := NoRefreshTokenSettingsURL(core.BrandFeishu, ""); got != "" {
		t.Fatalf("empty app id URL = %q, want empty", got)
	}
	if got := NoRefreshTokenSettingsURL(core.BrandFeishu, "a/b?c"); strings.Contains(got, "?") || strings.Contains(got, "a/b") {
		t.Fatalf("unescaped app id in %q", got)
	}
}

func TestNoRefreshTokenFixNamesSettingAndPublish(t *testing.T) {
	fix := NoRefreshTokenFix(core.BrandFeishu, "cli_abc")
	for _, want := range []string{`"Refresh user_access_token"`, "publish a new app version", "https://open.feishu.cn/app/cli_abc/safe"} {
		if !strings.Contains(fix, want) {
			t.Fatalf("fix = %q, want substring %q", fix, want)
		}
	}
}
