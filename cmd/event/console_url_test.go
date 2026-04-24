// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"testing"

	"github.com/larksuite/cli/internal/core"
)

func TestConsoleScopeGrantURL_Feishu(t *testing.T) {
	got := consoleScopeGrantURL(core.BrandFeishu, "cli_XXXXXXXXXXXXXXXX", []string{
		"im:message:readonly",
		"im:message.group_at_msg",
	})
	want := "https://open.feishu.cn/app/cli_XXXXXXXXXXXXXXXX/auth?q=im:message:readonly,im:message.group_at_msg&op_from=openapi&token_type=tenant"
	if got != want {
		t.Errorf("url\n got: %s\nwant: %s", got, want)
	}
}

func TestConsoleScopeGrantURL_LarkBrand(t *testing.T) {
	got := consoleScopeGrantURL(core.BrandLark, "cli_x", []string{"im:message"})
	want := "https://open.larksuite.com/app/cli_x/auth?q=im:message&op_from=openapi&token_type=tenant"
	if got != want {
		t.Errorf("url\n got: %s\nwant: %s", got, want)
	}
}

func TestConsoleScopeGrantURL_EmptyBrandDefaultsFeishu(t *testing.T) {
	got := consoleScopeGrantURL("", "cli_x", []string{"im:message"})
	if got != "https://open.feishu.cn/app/cli_x/auth?q=im:message&op_from=openapi&token_type=tenant" {
		t.Errorf("unexpected url: %s", got)
	}
}
