// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// TestResolveRegisterAuthMethod covers the non-interactive gating paths. No TEE
// signer is registered in this test binary, so private_key_jwt must be rejected.
func TestResolveRegisterAuthMethod(t *testing.T) {
	f := &cmdutil.Factory{}

	if m, err := resolveRegisterAuthMethod(f, core.AuthMethodClientSecret); err != nil || m != core.AuthMethodClientSecret {
		t.Errorf("client_secret: got (%q, %v), want (client_secret, nil)", m, err)
	}

	if _, err := resolveRegisterAuthMethod(f, "bogus"); err == nil {
		t.Error("bogus auth-method: expected error")
	}

	if _, err := resolveRegisterAuthMethod(f, core.AuthMethodPrivateKeyJWT); err == nil {
		t.Error("private_key_jwt without a signer: expected error")
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
	if m := resolveFinalAuthMethod(nil, ""); m != core.AuthMethodClientSecret {
		t.Errorf("default to client_secret: got %q", m)
	}
}
