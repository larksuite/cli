// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keylesshelper"
	"github.com/larksuite/cli/internal/keylessprovider"
	"github.com/larksuite/cli/internal/keysigner"
)

// ClientAuth describes how to authenticate the OAuth client at the token
// endpoint: with a client_secret (default) or a TEE-signed client_assertion
// (private_key_jwt).
type ClientAuth struct {
	AppID       string
	AppSecret   string
	AuthMethod  string // "" == client_secret; core.AuthMethodPrivateKeyJWT
	Signer      keysigner.Signer
	KeyLabel    string
	KeyProvider string

	// externalSigner is a verified provider snapshot prepared once for a
	// multi-request operation (for example a device-flow poll loop). The helper
	// still re-verifies its binary and mints a fresh assertion on every call.
	externalSigner clientAssertionSigner
}

type clientAssertionSigner interface {
	SignClientAssertion(context.Context, string, string, string) (string, string, error)
}

var resolveExternalAssertionSigner = func(ctx context.Context, provider string) (clientAssertionSigner, error) {
	return keylessprovider.Resolve(ctx, provider)
}

// ClientAuthFromConfig builds a ClientAuth from resolved config, picking up the
// active key signer for private_key_jwt apps.
func ClientAuthFromConfig(cfg *core.CliConfig) ClientAuth {
	if cfg == nil {
		return ClientAuth{}
	}
	return ClientAuth{
		AppID:       cfg.AppID,
		AppSecret:   cfg.AppSecret,
		AuthMethod:  cfg.AuthMethod,
		KeyLabel:    cfg.KeyLabel,
		KeyProvider: cfg.KeyProvider,
		Signer:      keysigner.Active(),
	}
}

func (c ClientAuth) isPrivateKeyJWT() bool { return c.AuthMethod == core.AuthMethodPrivateKeyJWT }

// ResolveSigner prepares the external private_key_jwt signer for reuse within
// one operation and returns the prepared copy. Built-in signers and
// client_secret authentication need no provider discovery. Keeping the
// resolved helper on ClientAuth separates expensive provider discovery from
// assertion minting: callers may reuse the returned value, while every call to
// applyClientAssertion still asks the signer for a fresh assertion.
func (c ClientAuth) ResolveSigner(ctx context.Context) (ClientAuth, error) {
	if !c.isPrivateKeyJWT() || c.KeyProvider == "" || c.externalSigner != nil {
		return c, nil
	}
	helper, err := resolveExternalAssertionSigner(ctx, c.KeyProvider)
	if err != nil {
		return c, err
	}
	if helper == nil {
		return c, fmt.Errorf("private_key_jwt provider %q resolved without a signer", c.KeyProvider)
	}
	c.externalSigner = helper
	return c, nil
}

// SignClientAssertion signs with a resolved external helper when present,
// otherwise with the platform signer.
func SignClientAssertion(ctx context.Context, signer keysigner.Signer, helper *keylesshelper.Command, keyLabel, clientID, audience string) (string, string, error) {
	if helper != nil {
		return helper.SignClientAssertion(ctx, keyLabel, clientID, audience)
	}
	assertion, err := jwt.SignClientAssertion(ctx, signer, keysigner.KeyRef{Label: keyLabel}, clientID, audience, time.Now())
	return jwt.ClientAssertionType, assertion, err
}

// applyClientAssertion adds client_assertion(+type) to a token-endpoint form for
// private_key_jwt and returns true. For client_secret it returns false, leaving
// the caller to apply its own secret-based authentication. audience is the token
// endpoint URL (the assertion's aud claim).
func (c ClientAuth) applyClientAssertion(ctx context.Context, form url.Values, audience string) (bool, error) {
	if !c.isPrivateKeyJWT() {
		return false, nil
	}
	var err error
	if c.KeyProvider != "" {
		c, err = c.ResolveSigner(ctx)
		if err != nil {
			return false, err
		}
	}
	helper := c.externalSigner
	if helper == nil && c.Signer == nil {
		return false, fmt.Errorf("private_key_jwt requires a key signer, but none is available on this build")
	}
	var assertionType, assertion string
	if helper != nil {
		assertionType, assertion, err = helper.SignClientAssertion(ctx, c.KeyLabel, c.AppID, audience)
	} else {
		assertionType, assertion, err = SignClientAssertion(ctx, c.Signer, nil, c.KeyLabel, c.AppID, audience)
	}
	if err != nil {
		return false, err
	}
	form.Set("client_assertion_type", assertionType)
	form.Set("client_assertion", assertion)
	return true, nil
}
