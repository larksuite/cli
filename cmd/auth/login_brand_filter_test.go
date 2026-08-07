// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"testing"

	"github.com/larksuite/cli/brand"
)

func TestBrandFilter_AppsExcludedOnLark(t *testing.T) {
	feishuDomains := allKnownDomains(brand.Feishu)
	if !feishuDomains["apps"] {
		t.Errorf("expected apps domain to be known on Feishu brand")
	}

	larkDomains := allKnownDomains(brand.Lark)
	if larkDomains["apps"] {
		t.Errorf("expected apps domain to be EXCLUDED on Lark brand")
	}

	feishuScopes := collectScopesForDomains([]string{"apps"}, "user", brand.Feishu)
	if len(feishuScopes) == 0 {
		t.Errorf("expected non-empty scopes for apps on Feishu brand, got %d", len(feishuScopes))
	}

	larkScopes := collectScopesForDomains([]string{"apps"}, "user", brand.Lark)
	if len(larkScopes) != 0 {
		t.Errorf("expected empty scopes for apps on Lark brand, got %d: %v", len(larkScopes), larkScopes)
	}
}
