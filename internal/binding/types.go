// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// OpenClawRoot captures the minimal subset of openclaw.json needed by config bind.
// Unknown fields are silently ignored (forward-compatible with future OpenClaw versions).
type OpenClawRoot struct {
	Channels ChannelsRoot   `json:"channels"`
	Secrets  *SecretsConfig `json:"secrets,omitempty"`
}

// ChannelsRoot holds channel configurations.
type ChannelsRoot struct {
	Feishu *FeishuChannel `json:"feishu,omitempty"`
}

// FeishuChannel represents the channels.feishu subtree.
// Single-account: AppID + AppSecret + Brand at top level.
// Multi-account: Accounts map (keyed by label like "work", "personal").
//
// Note: OpenClaw's canonical schema stores the brand under the key
// `domain` (values "feishu" | "lark"), not `brand`. The Go field name
// `Brand` stays aligned with our internal terminology, but the JSON
// tag matches OpenClaw's on-disk format.
type FeishuChannel struct {
	Enabled   *bool                     `json:"enabled,omitempty"` // nil = default enabled
	AppID     string                    `json:"appId,omitempty"`
	AppSecret SecretInput               `json:"appSecret,omitempty"`
	Brand     string                    `json:"domain,omitempty"`
	Accounts  map[string]*FeishuAccount `json:"accounts,omitempty"`
}

// FeishuAccount is a single account entry within Accounts.
// Like FeishuChannel, `Brand` maps to OpenClaw's `domain` key.
type FeishuAccount struct {
	Enabled   *bool       `json:"enabled,omitempty"` // nil = default enabled
	AppID     string      `json:"appId,omitempty"`
	AppSecret SecretInput `json:"appSecret,omitempty"`
	Brand     string      `json:"domain,omitempty"`
}

// isEnabled returns true if the enabled field is nil (default) or explicitly true.
func isEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

// SecretInput is a union type: either a plain string or a SecretRef object.
// Implements custom JSON unmarshaling to handle both forms.
type SecretInput struct {
	Plain string     // non-empty when value is a plain string (including "${VAR}" templates)
	Ref   *SecretRef // non-nil when value is a SecretRef object
}

// IsZero returns true if no value was provided.
func (s SecretInput) IsZero() bool {
	return s.Plain == "" && s.Ref == nil
}

// IsPlain returns true if this is a plain string (not a SecretRef object).
func (s SecretInput) IsPlain() bool {
	return s.Ref == nil
}

// SecretRef references a secret stored externally via OpenClaw's provider system.
type SecretRef struct {
	Source   string `json:"source"`             // "env" | "file" | "exec"
	Provider string `json:"provider,omitempty"` // provider alias; defaults to config.secrets.defaults.<source> or "default"
	ID       string `json:"id"`                 // lookup key (env var name / JSON pointer / exec ref id)
}

// validSources lists accepted SecretRef source values.
var validSources = map[string]bool{
	"env":  true,
	"file": true,
	"exec": true,
}

// EnvTemplateRe matches OpenClaw env template strings like "${FEISHU_APP_SECRET}".
// Only uppercase letters, digits, and underscores; 1-128 chars; must start with uppercase.
var EnvTemplateRe = regexp.MustCompile(`^\$\{([A-Z][A-Z0-9_]{0,127})\}$`)

// UnmarshalJSON handles both string and object forms of SecretInput.
func (s *SecretInput) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		s.Plain = str
		s.Ref = nil
		return nil
	}

	// Try SecretRef object
	var ref SecretRef
	if err := json.Unmarshal(data, &ref); err == nil {
		if !validSources[ref.Source] {
			return fmt.Errorf("SecretRef.source must be env|file|exec, got %q", ref.Source)
		}
		if ref.ID == "" {
			return fmt.Errorf("SecretRef.id must be non-empty")
		}
		s.Ref = &ref
		s.Plain = ""
		return nil
	}

	return fmt.Errorf("appSecret must be a string or {source, provider?, id} object")
}

// MarshalJSON serializes SecretInput back to JSON.
func (s SecretInput) MarshalJSON() ([]byte, error) {
	if s.Ref != nil {
		return json.Marshal(s.Ref)
	}
	return json.Marshal(s.Plain)
}

// SecretsConfig captures the secrets.providers registry from openclaw.json.
type SecretsConfig struct {
	Providers map[string]*ProviderConfig `json:"providers,omitempty"`
	Defaults  *ProviderDefaults          `json:"defaults,omitempty"`
}

// ProviderDefaults holds default provider aliases for each source type.
type ProviderDefaults struct {
	Env  string `json:"env,omitempty"`
	File string `json:"file,omitempty"`
	Exec string `json:"exec,omitempty"`
}

// DefaultProviderAlias is the fallback provider name when none is specified.
const DefaultProviderAlias = "default"

// ProviderConfig holds configuration for a secret provider.
// Fields are source-specific; unused fields for other sources are ignored.
type ProviderConfig struct {
	Source string `json:"source"` // "env" | "file" | "exec"

	// env source fields
	Allowlist []string `json:"allowlist,omitempty"`

	// file source fields
	Path      string `json:"path,omitempty"`
	Mode      string `json:"mode,omitempty"` // "singleValue" | "json"; default "json"
	TimeoutMs int    `json:"timeoutMs,omitempty"`
	MaxBytes  int    `json:"maxBytes,omitempty"`

	// exec source fields
	Command             string            `json:"command,omitempty"`
	Args                []string          `json:"args,omitempty"`
	NoOutputTimeoutMs   int               `json:"noOutputTimeoutMs,omitempty"`
	MaxOutputBytes      int               `json:"maxOutputBytes,omitempty"`
	JSONOnly            *bool             `json:"jsonOnly,omitempty"` // nil = default true
	Env                 map[string]string `json:"env,omitempty"`
	PassEnv             []string          `json:"passEnv,omitempty"`
	TrustedDirs         []string          `json:"trustedDirs,omitempty"`
	AllowInsecurePath   bool              `json:"allowInsecurePath,omitempty"`
	AllowSymlinkCommand bool              `json:"allowSymlinkCommand,omitempty"`
}

// Default values for provider config fields (aligned with OpenClaw resolve.ts).
const (
	DefaultFileTimeoutMs      = 5000
	DefaultFileMaxBytes       = 1024 * 1024 // 1 MiB
	DefaultExecTimeoutMs      = 10000
	DefaultExecMaxOutputBytes = 1024 * 1024 // 1 MiB
)

// ResolveDefaultProvider returns the effective provider alias for a SecretRef.
// If ref.Provider is set, returns it; otherwise falls back to config defaults or "default".
func ResolveDefaultProvider(ref *SecretRef, cfg *SecretsConfig) string {
	if ref.Provider != "" {
		return ref.Provider
	}
	if cfg != nil && cfg.Defaults != nil {
		switch ref.Source {
		case "env":
			if cfg.Defaults.Env != "" {
				return cfg.Defaults.Env
			}
		case "file":
			if cfg.Defaults.File != "" {
				return cfg.Defaults.File
			}
		case "exec":
			if cfg.Defaults.Exec != "" {
				return cfg.Defaults.Exec
			}
		}
	}
	return DefaultProviderAlias
}

// LookupProvider resolves a provider config from the registry.
// Returns the provider config or an error if not found.
// Special case: env source with "default" provider returns a synthetic empty env provider.
func LookupProvider(ref *SecretRef, cfg *SecretsConfig) (*ProviderConfig, error) {
	providerName := ResolveDefaultProvider(ref, cfg)

	if cfg != nil && cfg.Providers != nil {
		if pc, ok := cfg.Providers[providerName]; ok {
			if pc == nil {
				return nil, fmt.Errorf("secret provider %q is configured as null", providerName)
			}
			if pc.Source != ref.Source {
				return nil, fmt.Errorf("secret provider %q has source %q but ref requests %q",
					providerName, pc.Source, ref.Source)
			}
			return pc, nil
		}
	}

	// Special case: default env provider (implicit, per OpenClaw resolve.ts)
	if ref.Source == "env" && providerName == DefaultProviderAlias {
		return &ProviderConfig{Source: "env"}, nil
	}

	return nil, fmt.Errorf("secret provider %q is not configured (ref: %s:%s:%s)",
		providerName, ref.Source, providerName, ref.ID)
}

// CandidateApp represents a bindable app from OpenClaw's feishu channel config.
type CandidateApp struct {
	Label     string
	AppID     string
	AppSecret SecretInput
	Brand     string
}

// ListCandidateApps enumerates all bindable (enabled) apps from a FeishuChannel.
// Disabled accounts (enabled: false) are filtered out.
func ListCandidateApps(ch *FeishuChannel) []CandidateApp {
	if ch == nil {
		return nil
	}
	if len(ch.Accounts) > 0 {
		apps := make([]CandidateApp, 0, len(ch.Accounts)+1)

		// When accounts exist AND top-level has its own appId+appSecret,
		// include the top-level as a "default" candidate — aligned with
		// openclaw-lark getLarkAccountIds() which adds DEFAULT_ACCOUNT_ID
		// when top-level credentials are present and no explicit "default" exists.
		hasDefault := false
		for label := range ch.Accounts {
			if strings.EqualFold(strings.TrimSpace(label), "default") {
				hasDefault = true
				break
			}
		}
		if !hasDefault && ch.AppID != "" && !ch.AppSecret.IsZero() && isEnabled(ch.Enabled) {
			apps = append(apps, CandidateApp{
				Label:     "default",
				AppID:     ch.AppID,
				AppSecret: ch.AppSecret,
				Brand:     ch.Brand,
			})
		}

		for label, acct := range ch.Accounts {
			if acct == nil || !isEnabled(acct.Enabled) {
				continue // skip disabled accounts
			}
			appID := acct.AppID
			if appID == "" {
				appID = ch.AppID // inherit from top-level
			}
			if appID == "" {
				continue // skip entries with no effective AppID
			}
			appSecret := acct.AppSecret
			if appSecret.IsZero() {
				appSecret = ch.AppSecret // inherit from top-level
			}
			brand := acct.Brand
			if brand == "" {
				brand = ch.Brand
			}
			apps = append(apps, CandidateApp{
				Label:     label,
				AppID:     appID,
				AppSecret: appSecret,
				Brand:     brand,
			})
		}
		return apps
	}

	// Single account at top level — check if channel itself is enabled
	if ch.AppID != "" && isEnabled(ch.Enabled) {
		return []CandidateApp{{
			Label:     "",
			AppID:     ch.AppID,
			AppSecret: ch.AppSecret,
			Brand:     ch.Brand,
		}}
	}

	return nil
}
