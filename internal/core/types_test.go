// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"context"
	"net/url"
	"reflect"
	"testing"
)

func TestCredentialSourceContext(t *testing.T) {
	ctx := WithCredentialSource(context.Background(), CredentialSourceEnv)
	if source, ok := CredentialSourceFromContext(ctx); !ok || source != CredentialSourceEnv {
		t.Fatalf("CredentialSourceFromContext() = %q, %t; want env, true", source, ok)
	}

	ctx = WithCredentialSource(ctx, CredentialSource("vault:prod"))
	if source, ok := CredentialSourceFromContext(ctx); ok || source != "" {
		t.Fatalf("CredentialSourceFromContext() = %q, %t; want empty, false", source, ok)
	}
}

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
	// The unified OAuth v3 Token Endpoint mints TAT on the accounts domain;
	// pin the default-brand host so a stray non-production domain revert is caught.
	if ep.Accounts != "https://accounts.feishu.cn" {
		t.Errorf("Accounts = %q, want accounts.feishu.cn for empty brand", ep.Accounts)
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

func TestParseBrand(t *testing.T) {
	cases := []struct {
		in   string
		want LarkBrand
	}{
		{"", BrandFeishu},
		{"feishu", BrandFeishu},
		{"lark", BrandLark},
		{"LARK", BrandLark},
		{" lark ", BrandLark},
		{"Lark", BrandLark},
		{"xyz", BrandFeishu},
	}
	for _, c := range cases {
		if got := ParseBrand(c.in); got != c.want {
			t.Errorf("ParseBrand(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestResolveEndpoints_NormalizesBrand locks the boundary invariant: the
// resolver normalizes its brand input, so historical config values with
// unusual casing or whitespace still resolve to their intended endpoints.
func TestResolveEndpoints_NormalizesBrand(t *testing.T) {
	for _, raw := range []string{"LARK", " lark ", "Lark"} {
		if got := ResolveEndpoints(LarkBrand(raw)).Open; got != "https://open.larksuite.com" {
			t.Errorf("ResolveEndpoints(%q).Open = %q, want the lark endpoint", raw, got)
		}
	}
	if got := ResolveEndpoints(LarkBrand("unexpected")).Open; got != "https://open.feishu.cn" {
		t.Errorf("ResolveEndpoints(unexpected).Open = %q, want the feishu default", got)
	}
}

func TestIsPlatformEndpointHost_ExactMatchOnly(t *testing.T) {
	for _, host := range []string{
		"open.feishu.cn",
		"accounts.feishu.cn",
		"mcp.feishu.cn",
		"applink.feishu.cn",
		"open.larksuite.com",
		"accounts.larksuite.com",
		"mcp.larksuite.com",
		"applink.larksuite.com",
	} {
		if !IsPlatformEndpointHost(host) {
			t.Errorf("IsPlatformEndpointHost(%q) = false, want true", host)
		}
	}

	for _, host := range []string{
		"example.com",
		"open.feishu.cn.example.com",
		"notopen.feishu.cn",
		"",
	} {
		if IsPlatformEndpointHost(host) {
			t.Errorf("IsPlatformEndpointHost(%q) = true, want false", host)
		}
	}
}

func TestIsPlatformEndpointHost_CoversEveryResolvedEndpoint(t *testing.T) {
	for _, brand := range []LarkBrand{BrandFeishu, BrandLark} {
		endpoints := reflect.ValueOf(ResolveEndpoints(brand))
		for i := 0; i < endpoints.NumField(); i++ {
			rawURL := endpoints.Field(i).String()
			parsed, err := url.Parse(rawURL)
			if err != nil {
				t.Fatalf("ResolveEndpoints(%q) field %d URL %q: %v", brand, i, rawURL, err)
			}
			if !IsPlatformEndpointHost(parsed.Hostname()) {
				t.Errorf("ResolveEndpoints(%q) field %d host %q is missing from the platform transport boundary", brand, i, parsed.Hostname())
			}
		}
	}
}

func TestIsPlatformEndpointURL_RequiresSecureStandardOrigin(t *testing.T) {
	if IsPlatformEndpointURL(nil) {
		t.Error("IsPlatformEndpointURL(nil) = true, want false")
	}
	uppercaseScheme := &url.URL{Scheme: "HTTPS", Host: "open.feishu.cn", Path: "/path"}
	if !IsPlatformEndpointURL(uppercaseScheme) {
		t.Error("IsPlatformEndpointURL() rejected uppercase HTTPS scheme")
	}

	for _, rawURL := range []string{
		"http://open.feishu.cn/path",
		"https://open.feishu.cn:8443/path",
		"https://open.feishu.cn.example.com/path",
	} {
		candidate, err := url.Parse(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		if IsPlatformEndpointURL(candidate) {
			t.Errorf("IsPlatformEndpointURL(%q) = true, want false", rawURL)
		}
	}

	for _, rawURL := range []string{
		"https://open.feishu.cn/path",
		"https://open.feishu.cn:443/path",
		"https://OPEN.FEISHU.CN/path",
	} {
		candidate, err := url.Parse(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		if !IsPlatformEndpointURL(candidate) {
			t.Errorf("IsPlatformEndpointURL(%q) = false, want true", rawURL)
		}
	}
}
