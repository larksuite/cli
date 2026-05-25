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

func TestResolveEndpoints_EnvOverride(t *testing.T) {
	t.Setenv(EnvOpenBaseURL, "https://open.example.internal/")
	t.Setenv(EnvAccountsBaseURL, "  https://accounts.example.internal  ")
	t.Setenv(EnvMCPBaseURL, "https://mcp.example.internal")
	t.Setenv(EnvAppLinkBaseURL, "https://applink.example.internal///")

	ep := ResolveEndpoints(BrandFeishu)
	if ep.Open != "https://open.example.internal" {
		t.Errorf("Open = %q, want env override", ep.Open)
	}
	if ep.Accounts != "https://accounts.example.internal" {
		t.Errorf("Accounts = %q, want env override (whitespace trimmed)", ep.Accounts)
	}
	if ep.MCP != "https://mcp.example.internal" {
		t.Errorf("MCP = %q, want env override", ep.MCP)
	}
	if ep.AppLink != "https://applink.example.internal" {
		t.Errorf("AppLink = %q, want env override (trailing slashes stripped)", ep.AppLink)
	}
}

func TestResolveEndpoints_EnvOverride_PartialKeepsBrandDefaults(t *testing.T) {
	t.Setenv(EnvOpenBaseURL, "https://open.example.internal")

	ep := ResolveEndpoints(BrandLark)
	if ep.Open != "https://open.example.internal" {
		t.Errorf("Open = %q, want env override", ep.Open)
	}
	if ep.Accounts != "https://accounts.larksuite.com" {
		t.Errorf("Accounts = %q, want lark default (no env set)", ep.Accounts)
	}
	if ep.MCP != "https://mcp.larksuite.com" {
		t.Errorf("MCP = %q, want lark default", ep.MCP)
	}
	if ep.AppLink != "https://applink.larksuite.com" {
		t.Errorf("AppLink = %q, want lark default", ep.AppLink)
	}
}

func TestResolveEndpoints_EnvOverride_EmptyIgnored(t *testing.T) {
	t.Setenv(EnvOpenBaseURL, "   ")

	ep := ResolveEndpoints(BrandFeishu)
	if ep.Open != "https://open.feishu.cn" {
		t.Errorf("Open = %q, want feishu default (whitespace-only env should be ignored)", ep.Open)
	}
}

func TestResolveEndpoints_EnvOverride_InvalidFallsBackAndWarns(t *testing.T) {
	cases := []struct {
		name, value string
	}{
		{"missing scheme", "fsopen.bytedance.net"},
		{"unsupported scheme", "ftp://fsopen.bytedance.net"},
		{"scheme only", "https://"},
		{"path only", "/open-apis"},
		{"garbage", "::::not a url::::"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvOpenBaseURL, tc.value)

			var warnedEnv, warnedValue string
			prev := endpointEnvWarner
			endpointEnvWarner = func(env, value string) {
				warnedEnv = env
				warnedValue = value
			}
			t.Cleanup(func() { endpointEnvWarner = prev })

			ep := ResolveEndpoints(BrandFeishu)
			if ep.Open != "https://open.feishu.cn" {
				t.Errorf("Open = %q, want feishu default after invalid override", ep.Open)
			}
			if warnedEnv != EnvOpenBaseURL || warnedValue != tc.value {
				t.Errorf("warn = (%q, %q), want (%q, %q)", warnedEnv, warnedValue, EnvOpenBaseURL, tc.value)
			}
		})
	}
}

func TestResolveEndpoints_EnvOverride_PathPrefixAllowed(t *testing.T) {
	t.Setenv(EnvOpenBaseURL, "https://gateway.example.internal/lark")

	ep := ResolveEndpoints(BrandFeishu)
	if ep.Open != "https://gateway.example.internal/lark" {
		t.Errorf("Open = %q, want path-prefixed override to be accepted", ep.Open)
	}
}

func TestResolveEndpoints_EnvOverride_HTTPAllowed(t *testing.T) {
	t.Setenv(EnvOpenBaseURL, "http://127.0.0.1:18443")

	ep := ResolveEndpoints(BrandFeishu)
	if ep.Open != "http://127.0.0.1:18443" {
		t.Errorf("Open = %q, want http override to be accepted (needed for local testing / inner-plain-HTTP fabrics)", ep.Open)
	}
}
