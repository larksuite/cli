package credential

import (
	"context"
	"fmt"
	"strings"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
)

// Account is the credential-layer view of the active runtime account.
// It intentionally mirrors only the resolved fields needed by runtime auth
// and identity selection, without exposing core.CliConfig as a dependency.
type Account struct {
	ProfileName         string
	AppID               string
	AppSecret           string
	Brand               core.LarkBrand
	DefaultAs           core.Identity
	UserOpenId          string
	UserName            string
	SupportedIdentities uint8
}

const runtimePlaceholderAppSecret = "__LARKSUITE_CLI_TOKEN_ONLY__"

// HasRealAppSecret reports whether secret is an actual app secret rather than
// an empty/token-only marker or the internal runtime placeholder.
func HasRealAppSecret(secret string) bool {
	return secret != "" && secret != runtimePlaceholderAppSecret
}

// RuntimeAppSecret returns the SDK-compatible app secret used at runtime.
// Token-only sources intentionally have no real secret; this helper injects a
// private placeholder so downstream SDK validation can proceed while callers
// still distinguish real secrets with HasRealAppSecret.
func RuntimeAppSecret(secret string) string {
	if HasRealAppSecret(secret) {
		return secret
	}
	return runtimePlaceholderAppSecret
}

func normalizeAccountAppSecret(secret string) string {
	if HasRealAppSecret(secret) {
		return secret
	}
	return extcred.NoAppSecret
}

// AccountFromCliConfig copies the resolved config view into a credential.Account.
func AccountFromCliConfig(cfg *core.CliConfig) *Account {
	if cfg == nil {
		return nil
	}
	return &Account{
		ProfileName:         cfg.ProfileName,
		AppID:               cfg.AppID,
		AppSecret:           normalizeAccountAppSecret(cfg.AppSecret),
		Brand:               cfg.Brand,
		DefaultAs:           cfg.DefaultAs,
		UserOpenId:          cfg.UserOpenId,
		UserName:            cfg.UserName,
		SupportedIdentities: cfg.SupportedIdentities,
	}
}

// ToCliConfig copies the credential-layer account into the downstream config shape.
func (a *Account) ToCliConfig() *core.CliConfig {
	if a == nil {
		return nil
	}
	return &core.CliConfig{
		ProfileName:         a.ProfileName,
		AppID:               a.AppID,
		AppSecret:           normalizeAccountAppSecret(a.AppSecret),
		Brand:               a.Brand,
		DefaultAs:           a.DefaultAs,
		UserOpenId:          a.UserOpenId,
		UserName:            a.UserName,
		SupportedIdentities: a.SupportedIdentities,
	}
}

// AccountProvider resolves app credentials.
// Returns nil, nil to indicate "I don't handle this, try next provider".
type AccountProvider interface {
	ResolveAccount(ctx context.Context) (*Account, error)
}

// TokenType distinguishes UAT from TAT.
// Uses string constants matching extension/credential.TokenType for zero-cost conversion.
type TokenType string

const (
	TokenTypeUAT TokenType = "uat" // User Access Token
	TokenTypeTAT TokenType = "tat" // Tenant Access Token
)

func (t TokenType) String() string { return string(t) }

// ParseTokenType converts a string to TokenType.
func ParseTokenType(s string) (TokenType, bool) {
	switch strings.ToLower(s) {
	case "uat":
		return TokenTypeUAT, true
	case "tat":
		return TokenTypeTAT, true
	default:
		return "", false
	}
}

// TokenSpec is the input to TokenProvider.ResolveToken.
type TokenSpec struct {
	Type  TokenType
	AppID string // identifies which app (multi-account); not sensitive
}

// TokenResult is the output of TokenProvider.ResolveToken.
type TokenResult struct {
	Token  string
	Scopes string // optional, space-separated; empty = skip scope pre-check
}

// IdentityHint is credential-layer guidance for resolving the effective identity.
type IdentityHint struct {
	DefaultAs core.Identity
	AutoAs    core.Identity
}

// TokenUnavailableError reports that no usable token was available.
type TokenUnavailableError struct {
	Source string
	Type   TokenType
}

func (e *TokenUnavailableError) Error() string {
	if e.Source != "" {
		return fmt.Sprintf("no %s available from credential source %q", e.Type, e.Source)
	}
	return fmt.Sprintf("no credential provider returned a token for %s", e.Type)
}

// MalformedTokenResultError reports that a source returned an invalid token payload.
type MalformedTokenResultError struct {
	Source string
	Type   TokenType
	Reason string
}

func (e *MalformedTokenResultError) Error() string {
	return fmt.Sprintf("credential source %q returned malformed %s token: %s", e.Source, e.Type, e.Reason)
}

// TokenProvider resolves a runtime access token.
// Top-level resolvers should return a non-nil token or an error.
// Chain participants may use nil, nil internally to indicate "try next source".
type TokenProvider interface {
	ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error)
}

// NewTokenSpec returns a TokenSpec with the token type automatically
// selected based on identity: TAT for bot, UAT for user.
func NewTokenSpec(identity core.Identity, appID string) TokenSpec {
	t := TokenTypeUAT
	if identity.IsBot() {
		t = TokenTypeTAT
	}
	return TokenSpec{Type: t, AppID: appID}
}
