// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package jwt builds compact JWS tokens signed by a keysigner.Signer.
//
// It deliberately depends only on the standard library plus the existing
// google/uuid dependency — no third-party JWT library is introduced, keeping
// go.mod free of new dependencies. The actual signing (and, for ECDSA, the
// ASN.1->r||s conversion) is delegated to the Signer implementation.
package jwt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/larksuite/cli/extension/keysigner"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// buildSignedJWT builds a compact JWS:
//
//	base64url(header).base64url(claims).base64url(signature)
//
// alg is written into the header (it is part of the signed input) and verified
// against the alg the signer reports, guarding against a header/key mismatch.
// typ defaults to "JWT": the server's client_assertion generalizedValidation
// REQUIRES `typ == "JWT"` (rejects otherwise with "malformed client assertion
// jwt"), even though the spec examples (§8.1/§8.2) show only alg.
func buildSignedJWT(ctx context.Context, signer keysigner.Signer, ref keysigner.KeyRef, alg string, header, claims map[string]any) (string, error) {
	if signer == nil {
		return "", fmt.Errorf("jwt: no signer available (private_key_jwt unsupported on this build)")
	}
	if header == nil {
		header = map[string]any{}
	}
	header["alg"] = alg
	if _, ok := header["typ"]; !ok {
		header["typ"] = "JWT"
	}

	hb, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("jwt: marshal header: %w", err)
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("jwt: marshal claims: %w", err)
	}

	signingInput := b64(hb) + "." + b64(cb)
	sig, gotAlg, err := signer.Sign(ctx, ref, []byte(signingInput))
	if err != nil {
		return "", fmt.Errorf("jwt: sign: %w", err)
	}
	if gotAlg != alg {
		return "", fmt.Errorf("jwt: signer alg %q does not match header alg %q", gotAlg, alg)
	}
	return signingInput + "." + b64(sig), nil
}

// newJTI returns a random unique token identifier.
func newJTI() string { return uuid.NewString() }

// attestationTTL bounds the attestation JWT's lifetime. The init nonce (60s,
// single-use) is the real anti-replay constraint; this is a modest margin for
// clock skew on top of the immediate init→sign→begin round-trip.
const attestationTTL = 2 * time.Minute

// attestationClaims builds the registration attestation claim set per the App
// Registration JWT spec: jti, iat, exp (all required) and the init-issued nonce.
func attestationClaims(nonce string, now time.Time) map[string]any {
	return map[string]any{
		"jti":   newJTI(),
		"iat":   now.Unix(),
		"exp":   now.Add(attestationTTL).Unix(),
		"nonce": nonce,
	}
}

// clientAssertionClaims builds an RFC 7523 client_assertion claim set used to
// mint tokens in place of client_secret. aud is the brand's token endpoint URL.
func clientAssertionClaims(clientID, aud string, now time.Time, ttl time.Duration) map[string]any {
	return map[string]any{
		"iss": clientID,
		"sub": clientID,
		"aud": aud,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
		"jti": newJTI(),
	}
}

// ClientAssertionType is the RFC 7523 client_assertion_type value used for JWT
// bearer client authentication at the token endpoint.
const ClientAssertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"

// defaultAssertionTTL bounds a client_assertion's lifetime.
const defaultAssertionTTL = 5 * time.Minute

// SignAttestation signs the registration attestation JWT. The public key is
// embedded in the JWS "jwk" header so the registration backend can bind it to
// the app during action=begin; the claims carry the server nonce as a
// proof-of-possession challenge.
func SignAttestation(ctx context.Context, signer keysigner.Signer, ref keysigner.KeyRef, nonce string, now time.Time) (string, error) {
	if signer == nil {
		return "", fmt.Errorf("jwt: no signer available (private_key_jwt unsupported on this build)")
	}
	pub, err := signer.EnsureKey(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("jwt: ensure key: %w", err)
	}
	alg, err := keysigner.AlgForKey(pub)
	if err != nil {
		return "", err
	}
	jwk, err := keysigner.PublicKeyJWK(pub)
	if err != nil {
		return "", err
	}
	return buildSignedJWT(ctx, signer, ref, alg, map[string]any{"jwk": jwk}, attestationClaims(nonce, now))
}

// SignClientAssertion mints a short-lived RFC 7523 client_assertion: it reads the
// registered key (it must already exist — bound at registration; a missing key is
// an error, not a reason to create a new unbound one), derives the JWS alg from
// the public key, and signs an assertion whose audience is the brand's Open API
// host. The server, holding the public key bound at registration, verifies it in
// place of client_secret. The assertion header carries only alg (no jwk/kid);
// the server locates the key via iss/sub = client_id.
//
// This is the model-independent glue: the assertion JWT is identical whether the
// server augments an existing grant (device_code/refresh_token) with client
// authentication or uses a dedicated jwt-bearer grant — only where the caller
// attaches it differs.
func SignClientAssertion(ctx context.Context, signer keysigner.Signer, ref keysigner.KeyRef, clientID, audience string, now time.Time) (string, error) {
	if signer == nil {
		return "", fmt.Errorf("jwt: no signer available (private_key_jwt unsupported on this build)")
	}
	pub, err := signer.PublicKey(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("jwt: public key: %w", err)
	}
	alg, err := keysigner.AlgForKey(pub)
	if err != nil {
		return "", err
	}
	return buildSignedJWT(ctx, signer, ref, alg, map[string]any{}, clientAssertionClaims(clientID, audience, now, defaultAssertionTTL))
}
