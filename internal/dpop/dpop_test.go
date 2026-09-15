// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/keysigner"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestProofIsIndependentlyVerifiableAndBoundToRequest(t *testing.T) {
	binding, signer := transportTestBinding(t)
	key := binding.Key()
	now := time.Unix(1700000000, 0)
	key.clock = NewClockWithState(fixedClock{now}, ClockState{OffsetMillis: 3600000})
	seen := map[string]bool{}
	for range 2 {
		proof, err := key.SignProof("post", "HTTPS://EXAMPLE.COM:443/a/%7e/b/../c?secret=query#fragment")
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(proof, ".")
		if len(parts) != 3 {
			t.Fatal("proof is not compact JWS")
		}
		var header struct {
			Typ string            `json:"typ"`
			Alg string            `json:"alg"`
			JWK map[string]string `json:"jwk"`
		}
		var claims struct {
			JTI string `json:"jti"`
			HTM string `json:"htm"`
			HTU string `json:"htu"`
			IAT int64  `json:"iat"`
		}
		headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(headerBytes, &header); err != nil {
			t.Fatal(err)
		}
		claimBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(claimBytes, &claims); err != nil {
			t.Fatal(err)
		}
		if header.Typ != "dpop+jwt" || header.Alg != "ES256" || len(header.JWK) != 4 || header.JWK["kty"] != "EC" || header.JWK["crv"] != "P-256" {
			t.Fatal("invalid header or non-public JWK fields")
		}
		x, err := base64.RawURLEncoding.DecodeString(header.JWK["x"])
		if err != nil {
			t.Fatal(err)
		}
		y, err := base64.RawURLEncoding.DecodeString(header.JWK["y"])
		if err != nil {
			t.Fatal(err)
		}
		if len(x) != 32 || len(y) != 32 {
			t.Fatal("JWK coordinates must be fixed-width")
		}
		public := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !public.Equal(&signer.key.PublicKey) {
			t.Fatal("proof embeds a different key")
		}
		sig, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if len(sig) != 64 || !ecdsa.Verify(public, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
			t.Fatal("stdlib rejected proof signature")
		}
		if claims.HTM != "POST" || claims.HTU != "https://example.com/a/~/c" || claims.IAT != now.Add(time.Hour).Unix() || claims.JTI == "" || seen[claims.JTI] {
			t.Fatalf("incorrect or reused claims: %+v", claims)
		}
		seen[claims.JTI] = true
		// The documented Lark profile intentionally omits ath. Assert this wire
		// shape directly without reusing the production claims type.
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(claimBytes, &fields); err != nil {
			t.Fatal(err)
		}
		if len(fields) != 4 {
			t.Fatalf("unexpected proof claims: %s", claimBytes)
		}
		canonical, err := json.Marshal(header.JWK)
		if err != nil {
			t.Fatal(err)
		}
		thumbprint := sha256.Sum256(canonical)
		got, err := key.Thumbprint()
		if err != nil || got != base64.RawURLEncoding.EncodeToString(thumbprint[:]) {
			t.Fatalf("noncanonical JWK thumbprint: %v", err)
		}
	}
}

func TestNormalizeHTUPreservesResourceIdentity(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://EXAMPLE.com:443", "https://example.com/"},
		{"http://example.com:80/A//B/", "http://example.com/A//B/"},
		{"https://example.com:8443/A?x=1#f", "https://example.com:8443/A"},
		{"https://example.com/a/%2e%2e/b/%2f/%3f/%23", "https://example.com/b/%2F/%3F/%23"},
		{"https://example.com/a/b/..", "https://example.com/a/"},
		{"https://example.com/../../x", "https://example.com/x"},
		{"https://example.com/%41%7a%30%2d%2e%5f%7e", "https://example.com/Az0-._~"},
		{"https://[2001:DB8::1]:443/a", "https://[2001:db8::1]/a"},
		{"https://[2001:db8::1]:8443/a", "https://[2001:db8::1]:8443/a"},
		{"https://example.com/a%252Fb", "https://example.com/a%252Fb"},
	} {
		got, err := NormalizeHTU(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("%q => %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, raw := range []string{"", "/relative", "https:///missing-host", "https://user:pass@example.com/a", "ftp://example.com/a", "https://example.com/%zz", "https://example.com:bad/a"} {
		if _, err := NormalizeHTU(raw); err == nil {
			t.Errorf("accepted unsafe URL %q", raw)
		}
	}
}

type wrongSignatureSigner struct {
	keysigner.Signer
	signature []byte
	algorithm string
}

func (s wrongSignatureSigner) Sign(context.Context, keysigner.KeyRef, []byte) ([]byte, string, error) {
	return s.signature, s.algorithm, nil
}

func TestProofRejectsSignerFailuresAndMalformedSignatures(t *testing.T) {
	binding, signer := transportTestBinding(t)
	cause := errors.New("signing denied")
	signer.signErr = cause
	if proof, err := binding.SignProof("GET", "https://example.com/"); proof != "" || !errors.Is(err, cause) {
		t.Fatalf("signing failure = %q, %v", proof, err)
	}
	for _, tc := range []struct {
		length    int
		algorithm string
	}{{63, "ES256"}, {65, "ES256"}, {64, "RS256"}} {
		key := newKey("key", &signer.key.PublicKey, wrongSignatureSigner{Signer: signer, signature: make([]byte, tc.length), algorithm: tc.algorithm}, NewClock(nil))
		if proof, err := key.SignProof("GET", "https://example.com/"); err == nil || proof != "" {
			t.Fatal("accepted incompatible signer output")
		}
	}
}

func TestRestoreBindingRejectsKeySubstitution(t *testing.T) {
	binding, signer := transportTestBinding(t)
	for _, tc := range []struct{ token, id, jkt string }{
		{"", binding.KeyID, binding.JKT}, {binding.Token, "other-key", binding.JKT},
		{binding.Token, binding.KeyID, "wrong-thumbprint"}, {binding.Token, binding.KeyID, ""},
	} {
		if _, err := RestoreBinding(tc.token, tc.id, tc.jkt, binding.Key()); err == nil {
			t.Fatal("accepted inconsistent binding")
		}
	}
	// Snapshot public coordinates; a backend must not mutate the key whose
	// thumbprint has already been bound to an issued token.
	signer.key.PublicKey.X.SetInt64(0)
	got, err := binding.Key().Thumbprint()
	if err != nil || got != binding.JKT {
		t.Fatalf("backend mutated bound public key: %v", err)
	}
}

func TestClockRestoreAndConcurrentProofTime(t *testing.T) {
	now := time.Unix(1700000000, 0)
	clock := NewClock(fixedClock{now})
	clock.SetServerTime(now.Add(-time.Hour), now)
	state := clock.State()
	reopened := NewClockWithState(fixedClock{now.Add(time.Minute)}, state)
	if !reopened.Now().Equal(now.Add(-59 * time.Minute)) {
		t.Fatal("reopen lost clock calibration")
	}
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				if i%2 == 0 {
					clock.RestoreState(state)
				} else {
					_ = clock.Now()
					_ = clock.State()
				}
			}
		}()
	}
	wg.Wait()
	clock.RestoreState(ClockState{})
	if !clock.Now().Equal(now) || clock.State() != (ClockState{}) {
		t.Fatal("zero token state did not clear stale key calibration")
	}
}
