// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"crypto"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keysigner"
)

type authMethodTestSigner struct {
	info     keysigner.HardwareInfo
	probeErr error
}

func (authMethodTestSigner) EnsureKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	return nil, nil
}

func (authMethodTestSigner) PublicKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	return nil, nil
}

func (authMethodTestSigner) Sign(context.Context, keysigner.KeyRef, []byte) ([]byte, string, error) {
	return nil, "", nil
}

func (s authMethodTestSigner) ProbeHardware(context.Context) (keysigner.HardwareInfo, error) {
	return s.info, s.probeErr
}

// TestResolveRegisterAuthMethod covers the non-interactive gating paths. The
// darwin keychain signer is compiled into every build, so the test cannot rely
// on the binary lacking a signer — it forces a known no-signer state for the
// rejection cases, then registers a stub for the success case.
func TestResolveRegisterAuthMethod(t *testing.T) {
	f := &cmdutil.Factory{}
	ctx := context.Background()
	prevSigner := keysigner.Active()
	keysigner.Register(nil)
	t.Cleanup(func() { keysigner.Register(prevSigner) })

	if m, err := resolveRegisterAuthMethod(ctx, f, core.AuthMethodClientSecret); err != nil || m != core.AuthMethodClientSecret {
		t.Errorf("client_secret: got (%q, %v), want (client_secret, nil)", m, err)
	}

	if m, err := resolveRegisterAuthMethod(ctx, f, ""); err != nil || m != core.AuthMethodClientSecret {
		t.Errorf("default: got (%q, %v), want (client_secret, nil)", m, err)
	}

	if _, err := resolveRegisterAuthMethod(ctx, f, "bogus"); err == nil {
		t.Error("bogus auth-method: expected error")
	}

	if _, err := resolveRegisterAuthMethod(ctx, f, core.AuthMethodPrivateKeyJWT); err == nil {
		t.Error("private_key_jwt without a signer: expected error")
	}

	keysigner.Register(authMethodTestSigner{info: keysigner.HardwareInfo{Backend: "tpm2", Available: true}})

	if m, err := resolveRegisterAuthMethod(ctx, f, core.AuthMethodPrivateKeyJWT); err != nil || m != core.AuthMethodPrivateKeyJWT {
		t.Errorf("private_key_jwt with signer: got (%q, %v), want (private_key_jwt, nil)", m, err)
	}

	f.IOStreams = &cmdutil.IOStreams{IsTerminal: true}
	if m, err := resolveRegisterAuthMethod(ctx, f, ""); err != nil || m != core.AuthMethodClientSecret {
		t.Errorf("default with terminal signer: got (%q, %v), want (client_secret, nil)", m, err)
	}
}

func TestResolveRegisterAuthMethod_PrivateKeyJWTRejectsUnavailableHardware(t *testing.T) {
	prevSigner := keysigner.Active()
	t.Cleanup(func() { keysigner.Register(prevSigner) })
	keysigner.Register(authMethodTestSigner{info: keysigner.HardwareInfo{
		Backend: "tpm2",
		Reason:  "open /dev/tpmrm0: permission denied",
	}})

	_, err := resolveRegisterAuthMethod(context.Background(), &cmdutil.Factory{}, core.AuthMethodPrivateKeyJWT)
	if err == nil {
		t.Fatal("private_key_jwt with unavailable signer hardware: expected error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error is not typed: %T %[1]v", err)
	}
	if problem.Category != errs.CategoryConfig || problem.Subtype != errs.SubtypeInvalidClient {
		t.Fatalf("problem = %s/%s, want config/invalid_client", problem.Category, problem.Subtype)
	}
	wantMessage := "this machine does not support --private_key_jwt"
	if problem.Message != wantMessage {
		t.Fatalf("message = %q, want %q", problem.Message, wantMessage)
	}
	if strings.Contains(problem.Message, "sks") || strings.Contains(problem.Message, "/dev/tpm") || strings.Contains(problem.Message, "tpm") || strings.Contains(problem.Message, "TEE") || strings.Contains(problem.Message, "Keychain") {
		t.Fatalf("message exposes backend detail: %q", problem.Message)
	}
	if !strings.Contains(problem.Hint, "omit --private_key_jwt") {
		t.Fatalf("hint = %q, want guidance to omit --private_key_jwt", problem.Hint)
	}
	if strings.Contains(problem.Hint, "fix the local signer") {
		t.Fatalf("hint exposes unnecessary signer recovery: %q", problem.Hint)
	}
}

func TestResolveRegisterAuthMethod_PrivateKeyJWTRejectsProbeError(t *testing.T) {
	probeErr := errors.New("probe exploded")
	prevSigner := keysigner.Active()
	t.Cleanup(func() { keysigner.Register(prevSigner) })
	keysigner.Register(authMethodTestSigner{
		info:     keysigner.HardwareInfo{Backend: "keychain"},
		probeErr: probeErr,
	})

	_, err := resolveRegisterAuthMethod(context.Background(), &cmdutil.Factory{}, core.AuthMethodPrivateKeyJWT)
	if err == nil {
		t.Fatal("private_key_jwt with probe error: expected error")
	}
	if !errors.Is(err, probeErr) {
		t.Fatalf("error does not preserve probe cause: %v", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error is not typed: %T %[1]v", err)
	}
	if problem.Category != errs.CategoryConfig || problem.Subtype != errs.SubtypeInvalidClient {
		t.Fatalf("problem = %s/%s, want config/invalid_client", problem.Category, problem.Subtype)
	}
	wantMessage := "this machine does not support --private_key_jwt"
	if problem.Message != wantMessage {
		t.Fatalf("message = %q, want %q", problem.Message, wantMessage)
	}
	if strings.Contains(problem.Message, "probe") || strings.Contains(problem.Message, "keychain signer") {
		t.Fatalf("message exposes probe detail: %q", problem.Message)
	}
	if !strings.Contains(problem.Hint, "omit --private_key_jwt") {
		t.Fatalf("hint = %q, want guidance to omit --private_key_jwt", problem.Hint)
	}
	if strings.Contains(problem.Hint, "fix the local signer") {
		t.Fatalf("hint exposes unnecessary signer recovery: %q", problem.Hint)
	}
}

func TestExistingAppRequiresSecret(t *testing.T) {
	if !existingAppRequiresSecret(core.AuthMethodClientSecret) {
		t.Error("client_secret existing app should require App Secret")
	}
	if existingAppRequiresSecret("") != true {
		t.Error("default existing app should require App Secret")
	}
	if existingAppRequiresSecret(core.AuthMethodPrivateKeyJWT) {
		t.Error("private_key_jwt existing app should not require App Secret")
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
