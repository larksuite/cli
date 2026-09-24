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
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keylesshelper"
	"github.com/larksuite/cli/internal/keylessprovider"
	"github.com/larksuite/cli/internal/keysigner"
)

// ClientAuth describes how to authenticate the OAuth client at the token
// endpoint: with a client_secret (default) or a signed client_assertion.
type ClientAuth struct {
	AppID       string
	AppSecret   string
	AuthMethod  string // "" == client_secret; private_key_jwt or private_key_jwt_local_keypair
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

// ResolveConfigSigner resolves the host-local credential signer required by cfg.
func ResolveConfigSigner(cfg *core.CliConfig, kc keychain.KeychainAccess) (keysigner.Signer, error) {
	if cfg == nil || !core.IsPrivateKeyJWTAuthMethod(cfg.AuthMethod) {
		return nil, nil
	}
	switch cfg.KeySource {
	case "", core.SecretSourceTEE:
		if cfg.KeyProvider == core.KeylessProviderLarkSuite {
			return nil, nil
		}
		return keylesshelper.ResolveSigner(cfg.KeyProvider, kc)
	case core.SecretSourceKeyFile:
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
		AppID:       cfg.AppID,
		AppSecret:   cfg.AppSecret,
		AuthMethod:  cfg.AuthMethod,
		KeyLabel:    cfg.KeyLabel,
		KeyProvider: cfg.KeyProvider,
		Signer:      signer,
	}
}

func (c ClientAuth) isPrivateKeyJWT() bool {
	return core.IsPrivateKeyJWTAuthMethod(c.AuthMethod)
}

// ResolveSigner prepares an external private-key JWT signer for reuse within
// one operation and returns the prepared copy. Built-in signers and
// client_secret authentication need no provider discovery. Keeping the
// resolved helper on ClientAuth separates expensive provider discovery from
// assertion minting: callers may reuse the returned value, while every call to
// applyClientAssertion still asks the signer for a fresh assertion.
func (c ClientAuth) ResolveSigner(ctx context.Context) (ClientAuth, error) {
	if !c.isPrivateKeyJWT() || c.KeyProvider == "" ||
		c.KeyProvider == keysigner.SoftwareSignerName ||
		keysigner.IsPlatformSignerName(c.KeyProvider) || c.externalSigner != nil {
		return c, nil
	}
	if c.KeyProvider != core.KeylessProviderLarkSuite {
		return c, fmt.Errorf("unsupported %s provider %q", c.AuthMethod, c.KeyProvider)
	}
	helper, err := resolveExternalAssertionSigner(ctx, c.KeyProvider)
	if err != nil {
		return c, err
	}
	if helper == nil {
		return c, fmt.Errorf("%s provider %q resolved without a signer", c.AuthMethod, c.KeyProvider)
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
	if signer == nil {
		return "", "", fmt.Errorf("private-key JWT requires a signer")
	}
	// Imported PEM keys have no managed lifecycle or shared native keychain.
	if _, ok := signer.(*keylesshelper.FileSigner); ok {
		assertion, err := jwt.SignClientAssertion(ctx, signer, keysigner.KeyRef{Label: keyLabel}, clientID, audience, time.Now())
		return jwt.ClientAssertionType, assertion, err
	}
	store := keylesshelper.NewKeyStoreWithSigner(nil, signer)
	assertion, err := store.SignClientAssertionContext(ctx, signer.Name(), keyLabel, clientID, audience, time.Now())
	return jwt.ClientAssertionType, assertion, err
}

// applyClientAssertion adds client_assertion(+type) to a token-endpoint form
// for either private-key JWT method and returns true. For client_secret it
// returns false, leaving the caller to apply its own secret-based
// authentication. audience is the assertion aud claim.
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
		return false, fmt.Errorf("%s requires a key signer, but none is available on this build", c.AuthMethod)
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
