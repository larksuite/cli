// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// endpointEnvWarner is where invalid-env-override warnings are written.
// Defaults to os.Stderr; overridden in tests.
var endpointEnvWarner = func(env, value string) {
	fmt.Fprintf(os.Stderr, "[lark-cli] [WARN] %s=%q is not a valid absolute URL (need http/https scheme and host); falling back to brand default\n", env, value)
}

// LarkBrand represents the Lark platform brand.
// "feishu" targets China-mainland, "lark" targets international.
// Any other string is treated as a custom base URL.
type LarkBrand string

// Environment variables that override the brand-default endpoint hosts.
// When set (non-empty), each replaces the corresponding field of the
// Endpoints struct returned by ResolveEndpoints, regardless of brand.
// Values must be absolute URLs without a trailing slash, e.g.
// "https://open.example.internal". Trailing slashes are stripped.
const (
	EnvOpenBaseURL     = "LARKSUITE_CLI_OPEN_BASE_URL"
	EnvAccountsBaseURL = "LARKSUITE_CLI_ACCOUNTS_BASE_URL"
	EnvMCPBaseURL      = "LARKSUITE_CLI_MCP_BASE_URL"
	EnvAppLinkBaseURL  = "LARKSUITE_CLI_APPLINK_BASE_URL"
)

const (
	BrandFeishu LarkBrand = "feishu"
	BrandLark   LarkBrand = "lark"
)

// ParseBrand normalizes a brand string to a LarkBrand constant.
// Unrecognized values default to BrandFeishu.
func ParseBrand(value string) LarkBrand {
	if value == "lark" {
		return BrandLark
	}
	return BrandFeishu
}

// Endpoints holds resolved endpoint URLs for different Lark services.
type Endpoints struct {
	Open     string // e.g. "https://open.feishu.cn"
	Accounts string // e.g. "https://accounts.feishu.cn"
	MCP      string // e.g. "https://mcp.feishu.cn"
	AppLink  string // e.g. "https://applink.feishu.cn"
}

// ResolveEndpoints resolves endpoint URLs based on brand.
// Each field can be overridden at runtime via the corresponding
// EnvOpenBaseURL / EnvAccountsBaseURL / EnvMCPBaseURL / EnvAppLinkBaseURL
// environment variable; env values win over brand defaults.
func ResolveEndpoints(brand LarkBrand) Endpoints {
	var ep Endpoints
	switch brand {
	case BrandLark:
		ep = Endpoints{
			Open:     "https://open.larksuite.com",
			Accounts: "https://accounts.larksuite.com",
			MCP:      "https://mcp.larksuite.com",
			AppLink:  "https://applink.larksuite.com",
		}
	default:
		ep = Endpoints{
			Open:     "https://open.feishu.cn",
			Accounts: "https://accounts.feishu.cn",
			MCP:      "https://mcp.feishu.cn",
			AppLink:  "https://applink.feishu.cn",
		}
	}
	applyEndpointEnvOverrides(&ep)
	return ep
}

func applyEndpointEnvOverrides(ep *Endpoints) {
	if v, ok := readEndpointEnv(EnvOpenBaseURL); ok {
		ep.Open = v
	}
	if v, ok := readEndpointEnv(EnvAccountsBaseURL); ok {
		ep.Accounts = v
	}
	if v, ok := readEndpointEnv(EnvMCPBaseURL); ok {
		ep.MCP = v
	}
	if v, ok := readEndpointEnv(EnvAppLinkBaseURL); ok {
		ep.AppLink = v
	}
}

// readEndpointEnv reads an endpoint-override env var. Returns (value, true)
// when the env var holds a valid absolute http/https URL; returns ("", false)
// when unset/whitespace-only. When set but malformed, warns on stderr and
// returns ("", false) so the brand default is used.
func readEndpointEnv(env string) (string, bool) {
	raw := os.Getenv(env)
	v := normalizeBaseURL(raw)
	if v == "" {
		return "", false
	}
	if !isValidBaseURL(v) {
		endpointEnvWarner(env, raw)
		return "", false
	}
	return v, true
}

// isValidBaseURL reports whether v is an absolute http/https URL with a host.
// A non-empty path is allowed (some deployments serve the API under a prefix).
func isValidBaseURL(v string) bool {
	if v == "" {
		return false
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "https" || u.Scheme == "http"
}

// normalizeBaseURL trims whitespace and any trailing slashes from a
// base-URL env value. Returns "" if the input is empty after trimming.
func normalizeBaseURL(v string) string {
	v = strings.TrimSpace(v)
	for strings.HasSuffix(v, "/") {
		v = strings.TrimSuffix(v, "/")
	}
	return v
}

// ResolveOpenBaseURL returns the Open API base URL for the given brand.
func ResolveOpenBaseURL(brand LarkBrand) string {
	return ResolveEndpoints(brand).Open
}
