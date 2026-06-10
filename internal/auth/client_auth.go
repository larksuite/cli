// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/larksuite/cli/extension/keysigner"
	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/core"
)

// ClientAuth describes how to authenticate the OAuth client at the token
// endpoint: with a client_secret (default) or a TEE-signed client_assertion
// (private_key_jwt).
type ClientAuth struct {
	AppID      string
	AppSecret  string
	AuthMethod string // "" == client_secret; core.AuthMethodPrivateKeyJWT
	Signer     keysigner.Signer
	KeyLabel   string
}

// ClientAuthFromConfig builds a ClientAuth from resolved config, picking up the
// active key signer for private_key_jwt apps.
func ClientAuthFromConfig(cfg *core.CliConfig) ClientAuth {
	if cfg == nil {
		return ClientAuth{}
	}
	return ClientAuth{
		AppID:      cfg.AppID,
		AppSecret:  cfg.AppSecret,
		AuthMethod: cfg.AuthMethod,
		KeyLabel:   cfg.KeyLabel,
		Signer:     keysigner.Active(),
	}
}

func (c ClientAuth) isPrivateKeyJWT() bool { return c.AuthMethod == core.AuthMethodPrivateKeyJWT }

// applyClientAssertion adds client_assertion(+type) to a token-endpoint form for
// private_key_jwt and returns true. For client_secret it returns false, leaving
// the caller to apply its own secret-based authentication. audience is the token
// endpoint URL (the assertion's aud claim).
func (c ClientAuth) applyClientAssertion(ctx context.Context, form url.Values, audience string) (bool, error) {
	if !c.isPrivateKeyJWT() {
		return false, nil
	}
	if c.Signer == nil {
		return false, fmt.Errorf("private_key_jwt requires a key signer, but none is available on this build")
	}
	assertion, err := jwt.SignClientAssertion(ctx, c.Signer, keysigner.KeyRef{Label: c.KeyLabel}, c.AppID, audience, time.Now())
	if err != nil {
		return false, err
	}
	form.Set("client_assertion_type", jwt.ClientAssertionType)
	form.Set("client_assertion", assertion)
	return true, nil
}
