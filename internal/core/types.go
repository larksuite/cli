// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

// LarkBrand represents the Lark platform brand.
// "feishu" targets China-mainland, "lark" targets international.
// Any other string is treated as a custom base URL.
type LarkBrand string

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
	Open     string `json:"open"`     // e.g. "https://open.feishu.cn"
	Accounts string `json:"accounts"` // e.g. "https://accounts.feishu.cn"
	MCP      string `json:"mcp"`      // e.g. "https://mcp.feishu.cn"
	AppLink  string `json:"applink"`  // e.g. "https://applink.feishu.cn"
}

// brandRegistry maps brand names to their endpoint defaults.
// Built-in brands (feishu, lark) are pre-registered via init.
// Custom brands can be added via RegisterBrand.
var brandRegistry = map[string]Endpoints{}

func init() {
	brandRegistry[string(BrandFeishu)] = Endpoints{
		Open:     "https://open.feishu.cn",
		Accounts: "https://accounts.feishu.cn",
		MCP:      "https://mcp.feishu.cn",
		AppLink:  "https://applink.feishu.cn",
	}
	brandRegistry[string(BrandLark)] = Endpoints{
		Open:     "https://open.larksuite.com",
		Accounts: "https://accounts.larksuite.com",
		MCP:      "https://mcp.larksuite.com",
		AppLink:  "https://applink.larksuite.com",
	}
}

// RegisterBrand adds or updates a brand's endpoint defaults.
// Partial overrides are merged on top of the "feishu" defaults.
func RegisterBrand(name string, ep Endpoints) {
	base := brandRegistry[string(BrandFeishu)]
	if name == string(BrandLark) {
		base = brandRegistry[string(BrandLark)]
	}
	brandRegistry[name] = MergeEndpointOverrides(base, &ep)
}

// MergeEndpointOverrides returns a copy of base with non-empty overrides applied.
func MergeEndpointOverrides(base Endpoints, overrides *Endpoints) Endpoints {
	if overrides == nil {
		return base
	}
	result := base
	if overrides.Open != "" {
		result.Open = overrides.Open
	}
	if overrides.Accounts != "" {
		result.Accounts = overrides.Accounts
	}
	if overrides.MCP != "" {
		result.MCP = overrides.MCP
	}
	if overrides.AppLink != "" {
		result.AppLink = overrides.AppLink
	}
	return result
}

// ResolveEndpoints resolves endpoint URLs based on brand.
func ResolveEndpoints(brand LarkBrand) Endpoints {
	if ep, ok := brandRegistry[string(brand)]; ok {
		return ep
	}
	return brandRegistry[string(BrandFeishu)]
}

// ResolveOpenBaseURL returns the Open API base URL for the given brand.
func ResolveOpenBaseURL(brand LarkBrand) string {
	return ResolveEndpoints(brand).Open
}
