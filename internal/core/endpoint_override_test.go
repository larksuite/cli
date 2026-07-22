// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import "testing"

func TestResolveEndpoints_OverrideWins(t *testing.T) {
	t.Setenv(EndpointDomainEnv, "feishu-boe.cn")

	// The override must beat brand resolution for every brand.
	for _, brand := range []LarkBrand{BrandFeishu, BrandLark} {
		ep := ResolveEndpoints(brand)
		if ep.Open != "https://open.feishu-boe.cn" {
			t.Errorf("brand %q: Open = %q, want https://open.feishu-boe.cn", brand, ep.Open)
		}
		if ep.Accounts != "https://accounts.feishu-boe.cn" {
			t.Errorf("brand %q: Accounts = %q", brand, ep.Accounts)
		}
		if ep.MCP != "https://mcp.feishu-boe.cn" {
			t.Errorf("brand %q: MCP = %q", brand, ep.MCP)
		}
		if ep.AppLink != "https://applink.feishu-boe.cn" {
			t.Errorf("brand %q: AppLink = %q", brand, ep.AppLink)
		}
	}
}

func TestResolveEndpoints_BlankOverrideFallsBack(t *testing.T) {
	t.Setenv(EndpointDomainEnv, "   ")

	if got := ResolveEndpoints(BrandFeishu).Open; got != "https://open.feishu.cn" {
		t.Errorf("Open = %q, want https://open.feishu.cn", got)
	}
	if got := ResolveEndpoints(BrandLark).Open; got != "https://open.larksuite.com" {
		t.Errorf("Open = %q, want https://open.larksuite.com", got)
	}
}

func TestResolveEndpoints_UnsetOverrideFallsBack(t *testing.T) {
	t.Setenv(EndpointDomainEnv, "")

	if got := ResolveOpenBaseURL(BrandFeishu); got != "https://open.feishu.cn" {
		t.Errorf("ResolveOpenBaseURL = %q, want https://open.feishu.cn", got)
	}
}
