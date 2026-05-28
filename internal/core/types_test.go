// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import "testing"

func TestResolveEndpoints_Feishu(t *testing.T) {
	ep := ResolveEndpoints(BrandFeishu)
	if ep.Open != "https://open.feishu.cn" {
		t.Errorf("Open = %q, want feishu.cn", ep.Open)
	}
	if ep.Accounts != "https://accounts.feishu.cn" {
		t.Errorf("Accounts = %q, want feishu.cn", ep.Accounts)
	}
	if ep.MCP != "https://mcp.feishu.cn" {
		t.Errorf("MCP = %q, want feishu.cn", ep.MCP)
	}
	if ep.AppLink != "https://applink.feishu.cn" {
		t.Errorf("AppLink = %q, want feishu.cn", ep.AppLink)
	}
}

func TestResolveEndpoints_Lark(t *testing.T) {
	ep := ResolveEndpoints(BrandLark)
	if ep.Open != "https://open.larksuite.com" {
		t.Errorf("Open = %q, want larksuite.com", ep.Open)
	}
	if ep.Accounts != "https://accounts.larksuite.com" {
		t.Errorf("Accounts = %q, want larksuite.com", ep.Accounts)
	}
	if ep.MCP != "https://mcp.larksuite.com" {
		t.Errorf("MCP = %q, want larksuite.com", ep.MCP)
	}
	if ep.AppLink != "https://applink.larksuite.com" {
		t.Errorf("AppLink = %q, want larksuite.com", ep.AppLink)
	}
}

func TestResolveEndpoints_EmptyDefaultsToFeishu(t *testing.T) {
	ep := ResolveEndpoints("")
	if ep.Open != "https://open.feishu.cn" {
		t.Errorf("Open = %q, want feishu.cn for empty brand", ep.Open)
	}
}

func TestResolveOpenBaseURL(t *testing.T) {
	if got := ResolveOpenBaseURL(BrandFeishu); got != "https://open.feishu.cn" {
		t.Errorf("ResolveOpenBaseURL(feishu) = %q", got)
	}
	if got := ResolveOpenBaseURL(BrandLark); got != "https://open.larksuite.com" {
		t.Errorf("ResolveOpenBaseURL(lark) = %q", got)
	}
}

func TestMergeEndpointOverrides(t *testing.T) {
	base := Endpoints{
		Open:     "https://open.feishu.cn",
		Accounts: "https://accounts.feishu.cn",
		MCP:      "https://mcp.feishu.cn",
		AppLink:  "https://applink.feishu.cn",
	}

	t.Run("nil overrides returns base", func(t *testing.T) {
		got := MergeEndpointOverrides(base, nil)
		if got != base {
			t.Errorf("got %v, want %v", got, base)
		}
	})

	t.Run("partial override", func(t *testing.T) {
		got := MergeEndpointOverrides(base, &Endpoints{Open: "https://proxy.example.com"})
		if got.Open != "https://proxy.example.com" {
			t.Errorf("Open = %q", got.Open)
		}
		if got.Accounts != "https://accounts.feishu.cn" {
			t.Errorf("Accounts = %q, want unchanged", got.Accounts)
		}
	})

	t.Run("full override", func(t *testing.T) {
		overrides := Endpoints{
			Open: "https://a.example.com", Accounts: "https://b.example.com",
			MCP: "https://c.example.com", AppLink: "https://d.example.com",
		}
		got := MergeEndpointOverrides(base, &overrides)
		if got != overrides {
			t.Errorf("got %v, want %v", got, overrides)
		}
	})
}

func TestRegisterBrand(t *testing.T) {
	RegisterBrand("staging", Endpoints{Open: "https://open-staging.feishu.cn"})
	defer delete(brandRegistry, "staging")

	ep := ResolveEndpoints("staging")
	if ep.Open != "https://open-staging.feishu.cn" {
		t.Errorf("Open = %q, want staging URL", ep.Open)
	}
	if ep.Accounts != "https://accounts.feishu.cn" {
		t.Errorf("Accounts = %q, want feishu default", ep.Accounts)
	}
}

func TestRegisterBrand_Full(t *testing.T) {
	RegisterBrand("proxy", Endpoints{
		Open: "https://api-proxy.example.com", Accounts: "https://acct-proxy.example.com",
		MCP: "https://mcp-proxy.example.com", AppLink: "https://applink-proxy.example.com",
	})
	defer delete(brandRegistry, "proxy")

	ep := ResolveEndpoints("proxy")
	if ep.Open != "https://api-proxy.example.com" {
		t.Errorf("Open = %q", ep.Open)
	}
	if ep.Accounts != "https://acct-proxy.example.com" {
		t.Errorf("Accounts = %q", ep.Accounts)
	}
}

func TestRegisterBrand_IgnoresBuiltIn(t *testing.T) {
	original := brandRegistry[string(BrandFeishu)]
	RegisterBrand("feishu", Endpoints{Open: "https://malicious.example.com"})
	RegisterBrand("lark", Endpoints{Open: "https://malicious.example.com"})
	RegisterBrand("", Endpoints{Open: "https://malicious.example.com"})

	if brandRegistry[string(BrandFeishu)] != original {
		t.Error("RegisterBrand should not overwrite built-in feishu brand")
	}
	if brandRegistry[string(BrandLark)].Open == "https://malicious.example.com" {
		t.Error("RegisterBrand should not overwrite built-in lark brand")
	}
}
