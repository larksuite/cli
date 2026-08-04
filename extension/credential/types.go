// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import "context"

// Brand represents the Lark platform brand.
type Brand string

const (
	BrandLark   Brand = "lark"
	BrandFeishu Brand = "feishu"
)

// NoAppSecret marks that a credential source does not provide a real app secret.
// Token-only sources should return this value instead of inventing placeholder text.
const NoAppSecret = ""

// Identity represents the caller identity type.
type Identity string

const (
	IdentityUser Identity = "user"
	IdentityBot  Identity = "bot"
	IdentityAuto Identity = "auto"
)

// IdentitySupport declares which identities a credential source can provide.
type IdentitySupport uint8

const (
	SupportsUser IdentitySupport = 1 << iota
	SupportsBot
	SupportsAll = SupportsUser | SupportsBot
)

// Has reports whether s includes the given flag.
func (s IdentitySupport) Has(flag IdentitySupport) bool { return s&flag != 0 }

// UserOnly returns true if only user identity is supported.
func (s IdentitySupport) UserOnly() bool { return s == SupportsUser }

// BotOnly returns true if only bot identity is supported.
func (s IdentitySupport) BotOnly() bool { return s == SupportsBot }

// Account holds resolved app credentials and configuration.
type Account struct {
	AppID       string
	AppSecret   string   // real app secret; empty or NoAppSecret means unavailable
	Brand       Brand    // BrandLark or BrandFeishu
	DefaultAs   Identity // IdentityUser / IdentityBot / IdentityAuto; empty = not set
	ProfileName string
	// OpenID is the optional user open_id hint. If a UAT is available, the
	// user_info API result takes precedence unless OpenIDVerified is set.
	OpenID              string
	SupportedIdentities IdentitySupport // zero = provider did not declare; treat as no restriction
	// OpenIDVerified marks OpenID as an identity assertion the provider has
	// already verified. When true and OpenID is non-empty, the CLI skips the
	// startup user_info verification call and uses OpenID as-is wherever the
	// resolved user identity is consumed (identity selection, whoami display,
	// stored-token lookup). The CLI does NOT re-verify the asserted value:
	// a mismatched assertion is the responsibility of the integrator that
	// manages the token supply, and whoami will display the injected,
	// unverified value on this path. Setting OpenIDVerified with an empty
	// OpenID is treated as unasserted and falls back to normal verification.
	//
	// Appended at the end of the struct so adding it does not shift the
	// positions of existing fields for unkeyed struct literals.
	OpenIDVerified bool
}

// Token holds a resolved access token and optional metadata.
type Token struct {
	Value  string
	Scopes string // space-separated; empty = skip scope pre-check
	Source string // e.g. "env:LARKSUITE_CLI_USER_ACCESS_TOKEN", "vault:addr"
}

// TokenType represents the kind of access token.
type TokenType string

const (
	TokenTypeUAT TokenType = "uat"
	TokenTypeTAT TokenType = "tat"
)

// TokenSpec describes what token is needed.
type TokenSpec struct {
	Type  TokenType
	AppID string
}

// BlockError is returned by a Provider to actively reject a request
// and prevent subsequent providers in the chain from being consulted.
type BlockError struct {
	Provider string
	Reason   string
}

func (e *BlockError) Error() string {
	return "blocked by " + e.Provider + ": " + e.Reason
}

// Provider is the unified interface for credential resolution.
//
// Flow control uses Go's native mechanisms:
//   - Handle: return &Account{...}, nil  or  return &Token{...}, nil
//   - Skip:   return nil, nil
//   - Block:  return nil, &BlockError{...}
type Provider interface {
	Name() string
	ResolveAccount(ctx context.Context) (*Account, error)
	ResolveToken(ctx context.Context, req TokenSpec) (*Token, error)
}
