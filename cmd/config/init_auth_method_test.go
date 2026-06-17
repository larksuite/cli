// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"crypto"
	"testing"

	"github.com/larksuite/cli/extension/keysigner"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

type authMethodTestSigner struct{}

func (authMethodTestSigner) EnsureKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	return nil, nil
}

func (authMethodTestSigner) PublicKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	return nil, nil
}

func (authMethodTestSigner) Sign(context.Context, keysigner.KeyRef, []byte) ([]byte, string, error) {
	return nil, "", nil
}

// TestResolveRegisterAuthMethod covers the non-interactive gating paths. No TEE
// signer is registered in this test binary, so private_key_jwt must be rejected.
func TestResolveRegisterAuthMethod(t *testing.T) {
	f := &cmdutil.Factory{}

	if m, err := resolveRegisterAuthMethod(f, core.AuthMethodClientSecret); err != nil || m != core.AuthMethodClientSecret {
		t.Errorf("client_secret: got (%q, %v), want (client_secret, nil)", m, err)
	}

	if m, err := resolveRegisterAuthMethod(f, ""); err != nil || m != core.AuthMethodClientSecret {
		t.Errorf("default: got (%q, %v), want (client_secret, nil)", m, err)
	}

	if _, err := resolveRegisterAuthMethod(f, "bogus"); err == nil {
		t.Error("bogus auth-method: expected error")
	}

	if _, err := resolveRegisterAuthMethod(f, core.AuthMethodPrivateKeyJWT); err == nil {
		t.Error("private_key_jwt without a signer: expected error")
	}

	prevSigner := keysigner.Active()
	keysigner.Register(authMethodTestSigner{})
	t.Cleanup(func() { keysigner.Register(prevSigner) })

	if m, err := resolveRegisterAuthMethod(f, core.AuthMethodPrivateKeyJWT); err != nil || m != core.AuthMethodPrivateKeyJWT {
		t.Errorf("private_key_jwt with signer: got (%q, %v), want (private_key_jwt, nil)", m, err)
	}
}

// TestValidatePKJWTKeyBinding covers the guard that rejects a registration
// resolving to private_key_jwt with no signing key bound (e.g. an existing
// secret-based app was selected on the confirm page).
func TestValidatePKJWTKeyBinding(t *testing.T) {
	if err := validatePKJWTKeyBinding(core.AuthMethodPrivateKeyJWT, ""); err == nil {
		t.Error("pkjwt with empty keyLabel: expected error")
	}
	if err := validatePKJWTKeyBinding(core.AuthMethodPrivateKeyJWT, "agent-key"); err != nil {
		t.Errorf("pkjwt with keyLabel: expected nil, got %v", err)
	}
	if err := validatePKJWTKeyBinding(core.AuthMethodClientSecret, ""); err != nil {
		t.Errorf("client_secret: expected nil, got %v", err)
	}
}

// TestResolveFinalAuthMethod locks the authoritative-method logic. The 2nd case
// is the real bug: we requested private_key_jwt but the server resolved to an
// existing client_secret app — we must persist client_secret, not pkjwt.
func TestResolveFinalAuthMethod(t *testing.T) {
	if m := resolveFinalAuthMethod([]string{"client_secret", "private_key_jwt"}, core.AuthMethodClientSecret); m != core.AuthMethodPrivateKeyJWT {
		t.Errorf("prefers private_key_jwt: got %q", m)
	}
	if m := resolveFinalAuthMethod([]string{"client_secret"}, core.AuthMethodPrivateKeyJWT); m != core.AuthMethodClientSecret {
		t.Errorf("server client_secret must override requested pkjwt: got %q", m)
	}
	if m := resolveFinalAuthMethod(nil, core.AuthMethodPrivateKeyJWT); m != core.AuthMethodPrivateKeyJWT {
		t.Errorf("fallback to requested when server is silent: got %q", m)
	}
	// Explicit empty slice (not just nil) also falls back to requested — the same
	// len()==0 back-compat allowance the init guard relies on to let private_key_jwt
	// proceed against an older server (see internal/auth
	// TestRequestAppRegistrationInit_EmptySupportedAuthMethods).
	if m := resolveFinalAuthMethod([]string{}, core.AuthMethodPrivateKeyJWT); m != core.AuthMethodPrivateKeyJWT {
		t.Errorf("empty []string should fall back to requested private_key_jwt: got %q", m)
	}
	if m := resolveFinalAuthMethod(nil, ""); m != core.AuthMethodClientSecret {
		t.Errorf("default to client_secret: got %q", m)
	}
}
