// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"time"
)

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
	AppID               string
	AppSecret           string   // real app secret; empty or NoAppSecret means unavailable
	Brand               Brand    // BrandLark or BrandFeishu
	DefaultAs           Identity // IdentityUser / IdentityBot / IdentityAuto; empty = not set
	ProfileName         string
	OpenID              string          // optional; if UAT is available, API result takes precedence
	SupportedIdentities IdentitySupport // zero = provider did not declare; treat as no restriction

	// CredentialKind declares which credential this account is governed by.
	// Empty means the provider did not classify the account and the legacy
	// UAT/TAT resolution applies. An account declaring CredentialAAT reuses
	// OpenID to carry the Agent Employee principal the provider has already
	// verified; there is no second field with the same meaning.
	CredentialKind CredentialKind
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

// CredentialKind identifies which credential backs a resolved Access. It is a
// closed set: a value outside these three is a contract violation, not an
// extension point.
type CredentialKind string

const (
	CredentialUAT CredentialKind = "uat"
	CredentialAAT CredentialKind = "aat"
	CredentialTAT CredentialKind = "tat"
)

// AccessSpec describes what access a caller needs. Callers declare only the
// identity the command runs as; which credential satisfies that identity is
// decided by the resolved account, not by the call site.
type AccessSpec struct {
	Identity Identity
	AppID    string
}

// Access is a resolved credential together with the facts a caller may rely
// on. Kind and OpenID are checked against the account before the value is
// used, so a provider cannot widen its own authority by mislabelling them.
//
// How much of that is checked depends on what the account claims. An account
// that names its principal (OpenID on Account) is asking the CLI to hold it to
// that identity, so every field is verified: the returned OpenID must be
// present and match, and ExpiresAt must be set and in the future — an Agent
// Employee credential has no refresh token, so a zero expiry is rejected as a
// contract violation rather than read as "never expires".
//
// An account that names no principal has delegated the credential's lifecycle
// to the provider. The CLI has nothing to compare against and does not invent a
// comparison: OpenID, ExpiresAt and Scopes may all be empty, the value is used
// as returned, and deciding when it is stale belongs to whoever issues it.
type Access struct {
	Value     string
	Kind      CredentialKind
	OpenID    string
	Scopes    string // space-separated; empty = skip scope pre-check
	ExpiresAt time.Time
}

// AccessProvider is the optional interface a Provider implements to return a
// credential and its verified subject in one step. Providers that only
// implement Provider keep working unchanged; an account declaring
// CredentialAAT requires this interface, because the legacy Token path cannot
// carry the subject facts an Agent Employee credential must be checked against.
type AccessProvider interface {
	ResolveAccess(ctx context.Context, req AccessSpec) (*Access, error)
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
