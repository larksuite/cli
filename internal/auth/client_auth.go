// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keylesshelper"
	"github.com/larksuite/cli/internal/keysigner"
)

// ClientAuth describes how to authenticate the OAuth client at the token
// endpoint: with a client_secret (default) or a signed client_assertion.
type ClientAuth struct {
	AppID      string
	AppSecret  string
	AuthMethod string // "" == client_secret; private_key_jwt_local_keypair
	Signer     keysigner.Signer
	KeyLabel   string
}

// ResolveConfigSigner resolves the host-local credential signer required by cfg.
func ResolveConfigSigner(cfg *core.CliConfig, kc keychain.KeychainAccess) (keysigner.Signer, error) {
	if cfg == nil || cfg.AuthMethod != core.AuthMethodPrivateKeyJWTLocalKeyPair {
		return nil, nil
	}
	switch cfg.KeySource {
	case core.KeySourceTEE:
		return keylesshelper.ResolveSigner(cfg.KeyProvider, kc)
	case core.KeySourceFile:
		return &keylesshelper.FileSigner{}, nil
	default:
		return nil, fmt.Errorf("unsupported key source %q", cfg.KeySource)
	}
}

// ClientAuthFromConfig builds a ClientAuth from resolved config using the
// invocation-scoped signer selected by the caller.
func ClientAuthFromConfig(cfg *core.CliConfig, signer keysigner.Signer) ClientAuth {
	if cfg == nil {
		return ClientAuth{}
	}
	return ClientAuth{
		AppID:      cfg.AppID,
		AppSecret:  cfg.AppSecret,
		AuthMethod: cfg.AuthMethod,
		KeyLabel:   cfg.KeyLabel,
		Signer:     signer,
	}
}

// ApplyClientAssertion adds client_assertion(+type) to a token-endpoint form
// for private_key_jwt_local_keypair and returns true. For client_secret it returns
// false, leaving the caller to apply its own secret-based
// authentication. audience is the assertion aud claim.
func (c ClientAuth) ApplyClientAssertion(ctx context.Context, form url.Values, audience string) (bool, error) {
	if c.AuthMethod != core.AuthMethodPrivateKeyJWTLocalKeyPair {
		return false, nil
	}
	if c.Signer == nil {
		return false, fmt.Errorf("%s requires a key signer, but none is available on this build", c.AuthMethod)
	}
	var assertion string
	var err error
	// Imported PEM keys have no managed lifecycle or shared native keychain.
	if file, ok := c.Signer.(*keylesshelper.FileSigner); ok {
		assertion, err = file.SignClientAssertion(ctx, keysigner.KeyRef{Label: c.KeyLabel}, c.AppID, audience, time.Now())
	} else {
		assertion, err = keylesshelper.SignClientAssertionContext(ctx, c.Signer, c.KeyLabel, c.AppID, audience, time.Now())
	}
	if err != nil {
		if _, typed := errs.ProblemOf(err); !typed && errors.Is(err, keysigner.ErrKeyNotFound) {
			err = errs.NewAuthenticationError(errs.SubtypeSigningKeyMissing,
				"application signing key is missing; cannot authenticate with private-key JWT").
				WithHint("run `lark-cli config init --private-key-jwt` in a terminal, choose the existing-app option, and enter app ID %s to re-register its signing key", c.AppID).
				WithCause(err)
		}
		return false, err
	}
	form.Set("client_assertion_type", jwt.ClientAssertionType)
	form.Set("client_assertion", assertion)
	return true, nil
}
