// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import "strings"

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
// "feishu" targets China-mainland, "lark" targets international.
// Any other string starting with "http" is treated as a custom base URL
// for private deployments (e.g. "https://your-company.feishu.cn").
func ResolveEndpoints(brand LarkBrand) Endpoints {
	switch brand {
	case BrandLark:
		return Endpoints{
			Open:     "https://open.larksuite.com",
			Accounts: "https://accounts.larksuite.com",
			MCP:      "https://mcp.larksuite.com",
		}
	case BrandFeishu, "":
		return Endpoints{
			Open:     "https://open.feishu.cn",
			Accounts: "https://accounts.feishu.cn",
			MCP:      "https://mcp.feishu.cn",
		}
	default:
		// Custom base URL for private deployments.
		// All services share the same domain; paths differ per API.
		base := strings.TrimRight(string(brand), "/")
		return Endpoints{
			Open:     base,
			Accounts: base,
			MCP:      base,
		}
	}
}

// ResolveOpenBaseURL returns the Open API base URL for the given brand.
func ResolveOpenBaseURL(brand LarkBrand) string {
	return ResolveEndpoints(brand).Open
}
