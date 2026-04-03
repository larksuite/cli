package credential

import (
	"context"
	"strings"

	"github.com/larksuite/cli/internal/core"
)

// Account is an alias for core.CliConfig — they carry the same fields.
type Account = core.CliConfig

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

// TokenProvider resolves a runtime access token.
// Returns nil, nil to indicate "I don't handle this, try next provider".
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
