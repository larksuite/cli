// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"fmt"
	"net/url"
	"strings"
)

// LarkBrand represents the Lark platform brand.
// "feishu" targets China-mainland, "lark" targets international.
// Any other string is treated as a custom base URL.
type LarkBrand string

const (
	BrandFeishu LarkBrand = "feishu"
	BrandLark   LarkBrand = "lark"
)

// Endpoints holds resolved endpoint URLs for different Lark services.
type Endpoints struct {
	Open     string // e.g. "https://open.feishu.cn"
	Accounts string // e.g. "https://accounts.feishu.cn"
	MCP      string // e.g. "https://mcp.feishu.cn"
}

// ResolveEndpoints resolves endpoint URLs based on brand.
// If brand is a URL (starts with http:// or https://), use it as custom domain.
func ResolveEndpoints(brand LarkBrand) Endpoints {
	// Check if brand is a custom URL
	if len(brand) > 8 && (brand[:7] == "http://" || brand[:8] == "https://") {
		// Custom domain: extract base URL and derive other endpoints
		baseURL := string(brand)
		// Remove trailing slash if present
		if baseURL[len(baseURL)-1] == '/' {
			baseURL = baseURL[:len(baseURL)-1]
		}

		// For custom deployments, typically:
		// - Open API: <domain>/open-apis
		// - Accounts: <domain>/accounts (or same domain)
		// - MCP: <domain>/mcp
		return Endpoints{
			Open:     baseURL,
			Accounts: baseURL, // Often same as Open for private deployments
			MCP:      baseURL, // Often same as Open for private deployments
		}
	}

	// Standard brands
	switch brand {
	case BrandLark:
		return Endpoints{
			Open:     "https://open.larksuite.com",
			Accounts: "https://accounts.larksuite.com",
			MCP:      "https://mcp.larksuite.com",
		}
	default:
		return Endpoints{
			Open:     "https://open.feishu.cn",
			Accounts: "https://accounts.feishu.cn",
			MCP:      "https://mcp.feishu.cn",
		}
	}
}

// ResolveOpenBaseURL returns the Open API base URL for the given brand.
func ResolveOpenBaseURL(brand LarkBrand) string {
	return ResolveEndpoints(brand).Open
}

// ResolveServiceEndpoints validates explicitly configured service bases without public fallback.
func (c *CliConfig) ResolveServiceEndpoints() (Endpoints, error) {
	if c.Brand != "" && c.Brand != BrandFeishu && c.Brand != BrandLark {
		legacy, e := url.Parse(string(c.Brand))
		if e != nil || legacy.Scheme != "https" || legacy.Host == "" || legacy.User != nil || legacy.RawQuery != "" || legacy.Fragment != "" {
			return Endpoints{}, fmt.Errorf("invalid private endpoint")
		}
	}
	if (c.Brand == "" || c.Brand == BrandFeishu || c.Brand == BrandLark) && ((c.APIBaseURL != "" && c.AuthorizationBaseURL == "") || (c.AuthorizationBaseURL != "" && c.APIBaseURL == "")) {
		return Endpoints{}, fmt.Errorf("explicit service bases require both API and authorization bases; no public fallback")
	}
	ep := ResolveEndpoints(c.Brand)
	if c.APIBaseURL != "" {
		ep.Open = c.APIBaseURL
	}
	if c.AuthorizationBaseURL != "" {
		ep.Accounts = c.AuthorizationBaseURL
	}
	if c.AuthProtocol != "" && c.AuthProtocol != "legacy_oidc" && c.AuthProtocol != "oauth_v2" {
		return Endpoints{}, fmt.Errorf("unsupported auth protocol")
	}
	if c.Brand != "" && c.Brand != BrandFeishu && c.Brand != BrandLark && !strings.HasPrefix(string(c.Brand), "https://") {
		return Endpoints{}, fmt.Errorf("invalid private endpoint: HTTPS required")
	}
	for _, base := range []string{ep.Open, ep.Accounts} {
		u, err := url.Parse(base)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return Endpoints{}, fmt.Errorf("invalid service endpoint: HTTPS base without credentials/query/fragment required")
		}
	}
	ep.Open = strings.TrimRight(ep.Open, "/")
	ep.Accounts = strings.TrimRight(ep.Accounts, "/")
	return ep, nil
}
