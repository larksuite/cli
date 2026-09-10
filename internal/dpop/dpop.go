// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package dpop implements the DPoP protocol primitives used by OAuth token
// requests and Lark OpenAPI requests. Key contains only a public key and a
// stable signer handle.
package dpop

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/larksuite/cli/internal/keysigner"
)

const (
	ProofHeader            = "DPoP"
	Scheme                 = "DPoP"
	TokenType              = "DPoP"
	ClockSkewErrorCode     = 1106072
	InvalidProofOAuthError = "invalid_dpop_proof"
)

// ErrInvalidProofResponse is a safe cause for typed errors derived from an
// invalid_dpop_proof Token Endpoint response. It intentionally excludes the
// response description, token, proof, and key material.
var ErrInvalidProofResponse = errors.New("token endpoint returned invalid_dpop_proof")

// IsClockRecoverySignal reports whether a Token Endpoint response carries the
// complete server signal that permits a one-time DPoP clock correction. The
// OAuth error comparison is deliberately exact.
func IsClockRecoverySignal(code int, oauthError string) bool {
	return code == ClockSkewErrorCode && oauthError == InvalidProofOAuthError
}

// Clock supplies timestamps for DPoP proofs and DPoP-bound token lifetimes.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// AdjustableClock is safe for concurrent proof generation and clock recovery.
type AdjustableClock struct {
	base     Clock
	mu       sync.RWMutex
	offset   time.Duration
	syncedAt time.Time
}

// ClockState is the non-secret persistent state needed to restore a calibrated
// clock with its platform key.
type ClockState struct {
	OffsetMillis   int64
	SyncedAtMillis int64
}

func NewClock(base Clock) *AdjustableClock {
	return NewClockWithState(base, ClockState{})
}

func NewClockWithState(base Clock, state ClockState) *AdjustableClock {
	if base == nil {
		base = systemClock{}
	}
	clock := &AdjustableClock{
		base:   base,
		offset: time.Duration(state.OffsetMillis) * time.Millisecond,
	}
	if state.SyncedAtMillis > 0 {
		clock.syncedAt = time.UnixMilli(state.SyncedAtMillis)
	}
	return clock
}

// RestoreState replaces the persisted calibration without reading the system
// clock. Token records can use this to remain the atomic source of truth for a
// UAT and its proof clock.
func (c *AdjustableClock) RestoreState(state ClockState) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.offset = time.Duration(state.OffsetMillis) * time.Millisecond
	c.syncedAt = time.Time{}
	if state.SyncedAtMillis > 0 {
		c.syncedAt = time.UnixMilli(state.SyncedAtMillis)
	}
	c.mu.Unlock()
}

func (c *AdjustableClock) Now() time.Time {
	if c == nil {
		return time.Now()
	}
	c.mu.RLock()
	offset := c.offset
	c.mu.RUnlock()
	return c.base.Now().Add(offset)
}

// SetServerTime records serverTime-localReceiveTime as the shared DPoP clock
// offset used by proofs and DPoP-bound token lifetime calculations.
func (c *AdjustableClock) SetServerTime(serverTime, localReceiveTime time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.offset = serverTime.Sub(localReceiveTime)
	c.syncedAt = localReceiveTime
	c.mu.Unlock()
}

func (c *AdjustableClock) State() ClockState {
	if c == nil {
		return ClockState{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	state := ClockState{OffsetMillis: c.offset.Milliseconds()}
	if !c.syncedAt.IsZero() {
		state.SyncedAtMillis = c.syncedAt.UnixMilli()
	}
	return state
}

// Key is a capability to sign with an ES256/P-256 key. It stores only the
// public key, a non-secret backend identifier, and the platform signer.
type Key struct {
	id            string
	public        *ecdsa.PublicKey
	signer        keysigner.Signer
	provider      string
	securityLevel keysigner.SecurityLevel
	clock         *AdjustableClock
}

func newKey(id string, public *ecdsa.PublicKey, signer keysigner.Signer, clock *AdjustableClock) *Key {
	return newKeyWithMetadata(id, public, signer, signer.Name(), signer.SecurityLevel(), clock)
}

func newKeyWithMetadata(id string, public *ecdsa.PublicKey, signer keysigner.Signer, provider string, securityLevel keysigner.SecurityLevel, clock *AdjustableClock) *Key {
	return &Key{
		id:            id,
		public:        clonePublicKey(public),
		signer:        signer,
		provider:      provider,
		securityLevel: securityLevel,
		clock:         clock,
	}
}

func (k *Key) ID() string {
	if k == nil {
		return ""
	}
	return k.id
}

func (k *Key) Clock() *AdjustableClock {
	if k == nil {
		return nil
	}
	return k.clock
}

func (k *Key) Provider() string {
	if k == nil {
		return ""
	}
	return k.provider
}

func (k *Key) SecurityLevel() keysigner.SecurityLevel {
	if k == nil {
		return ""
	}
	return k.securityLevel
}

func (k *Key) PublicJWK() (keysigner.PublicJWK, error) {
	if k == nil || k.public == nil || k.public.Curve != elliptic.P256() || k.public.X == nil || k.public.Y == nil ||
		!k.public.Curve.IsOnCurve(k.public.X, k.public.Y) {
		return keysigner.PublicJWK{}, errors.New("DPoP key is unavailable or is not P-256")
	}
	return keysigner.PublicKeyJWK(k.public)
}

func clonePublicKey(public *ecdsa.PublicKey) *ecdsa.PublicKey {
	if public == nil || public.X == nil || public.Y == nil {
		return nil
	}
	return &ecdsa.PublicKey{
		Curve: public.Curve,
		X:     new(big.Int).Set(public.X),
		Y:     new(big.Int).Set(public.Y),
	}
}

// Thumbprint computes the RFC 7638 SHA-256 JWK thumbprint. The member order is
// deliberately fixed to the RFC-required canonical JSON for an EC key.
func (k *Key) Thumbprint() (string, error) {
	jwk, err := k.PublicJWK()
	if err != nil {
		return "", err
	}
	canonical := fmt.Sprintf(`{"crv":"%s","kty":"%s","x":"%s","y":"%s"}`,
		jwk.Crv, jwk.Kty, jwk.X, jwk.Y)
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

type proofHeader struct {
	Typ string              `json:"typ"`
	Alg string              `json:"alg"`
	JWK keysigner.PublicJWK `json:"jwk"`
}

type proofClaims struct {
	JTI string `json:"jti"`
	HTM string `json:"htm"`
	HTU string `json:"htu"`
	IAT int64  `json:"iat"`
}

// SignProof creates a one-use compact JWS.
func (k *Key) SignProof(method, rawURL string) (string, error) {
	return k.SignProofContext(context.Background(), method, rawURL)
}

// SignProofContext creates a one-use proof while preserving request
// cancellation through the platform signing boundary.
func (k *Key) SignProofContext(ctx context.Context, method, rawURL string) (string, error) {
	if k == nil || k.public == nil || k.signer == nil || k.id == "" {
		return "", errors.New("DPoP key is unavailable")
	}
	htu, err := NormalizeHTU(rawURL)
	if err != nil {
		return "", err
	}
	jwk, err := k.PublicJWK()
	if err != nil {
		return "", err
	}
	claims := proofClaims{
		JTI: uuid.NewString(),
		HTM: strings.ToUpper(method),
		HTU: htu,
		IAT: k.clock.Now().Unix(),
	}
	headerJSON, err := json.Marshal(proofHeader{Typ: "dpop+jwt", Alg: keysigner.AlgES256, JWK: jwk})
	if err != nil {
		return "", fmt.Errorf("encode DPoP header: %w", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("encode DPoP claims: %w", err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := encodedHeader + "." + encodedClaims
	signature, algorithm, err := k.signer.Sign(ctx, keysigner.KeyRef{Label: k.id, Algorithm: keysigner.AlgES256}, []byte(signingInput))
	if err != nil {
		return "", fmt.Errorf("sign DPoP proof: %w", err)
	}
	if algorithm != keysigner.AlgES256 || len(signature) != 64 {
		return "", fmt.Errorf("sign DPoP proof: signer returned algorithm %q and %d-byte signature", algorithm, len(signature))
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// NormalizeHTU produces the RFC 3986 form used by DPoP htu: scheme and host
// are lowercase, default ports/query/fragment/dot segments are removed, and
// percent encodings are canonicalized without changing path case.
func NormalizeHTU(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("invalid DPoP request URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("invalid DPoP request URL scheme %q", parsed.Scheme)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", errors.New("invalid DPoP request URL host")
	}
	port := parsed.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	authority := host
	if strings.Contains(host, ":") {
		authority = "[" + host + "]"
	}
	if port != "" {
		authority = net.JoinHostPort(host, port)
	}
	escapedPath := parsed.EscapedPath()
	if escapedPath == "" {
		escapedPath = "/"
	}
	escapedPath, err = normalizePercentEncoding(escapedPath)
	if err != nil {
		return "", fmt.Errorf("invalid DPoP request URL path: %w", err)
	}
	escapedPath = removeDotSegments(escapedPath)
	if escapedPath == "" {
		escapedPath = "/"
	}
	return scheme + "://" + authority + escapedPath, nil
}

func normalizePercentEncoding(value string) (string, error) {
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] != '%' {
			out.WriteByte(value[i])
			continue
		}
		if i+2 >= len(value) {
			return "", errors.New("truncated percent encoding")
		}
		high, okHigh := hexValue(value[i+1])
		low, okLow := hexValue(value[i+2])
		if !okHigh || !okLow {
			return "", errors.New("invalid percent encoding")
		}
		decoded := high<<4 | low
		if isUnreserved(decoded) {
			out.WriteByte(decoded)
		} else {
			const upperHex = "0123456789ABCDEF"
			out.WriteByte('%')
			out.WriteByte(upperHex[decoded>>4])
			out.WriteByte(upperHex[decoded&0x0f])
		}
		i += 2
	}
	return out.String(), nil
}

func hexValue(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

func isUnreserved(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' ||
		value == '-' || value == '.' || value == '_' || value == '~'
}

// removeDotSegments implements RFC 3986 section 5.2.4 without collapsing
// ordinary empty path segments.
func removeDotSegments(input string) string {
	var output string
	for input != "" {
		switch {
		case strings.HasPrefix(input, "../"):
			input = input[3:]
		case strings.HasPrefix(input, "./"):
			input = input[2:]
		case strings.HasPrefix(input, "/./"):
			input = "/" + input[3:]
		case input == "/.":
			input = "/"
		case strings.HasPrefix(input, "/../"):
			input = "/" + input[4:]
			output = removeLastPathSegment(output)
		case input == "/..":
			input = "/"
			output = removeLastPathSegment(output)
		case input == "." || input == "..":
			input = ""
		default:
			next := len(input)
			start := 0
			if input[0] == '/' {
				start = 1
			}
			if index := strings.IndexByte(input[start:], '/'); index >= 0 {
				next = start + index
			}
			output += input[:next]
			input = input[next:]
		}
	}
	return output
}

func removeLastPathSegment(path string) string {
	if index := strings.LastIndexByte(path, '/'); index >= 0 {
		return path[:index]
	}
	return ""
}

// Binding associates a token with the only key that may prove possession of it.
type Binding struct {
	Token               string
	KeyID               string
	JKT                 string
	KeyStoreSecureLevel keysigner.SecurityLevel
	key                 *Key
}

func NewBinding(token string, key *Key) (*Binding, error) {
	if token == "" || key == nil {
		return nil, errors.New("DPoP binding requires a token and key")
	}
	jkt, err := key.Thumbprint()
	if err != nil {
		return nil, err
	}
	return &Binding{
		Token:               token,
		KeyID:               key.ID(),
		JKT:                 jkt,
		KeyStoreSecureLevel: key.SecurityLevel(),
		key:                 key,
	}, nil
}

func RestoreBinding(token, keyID, expectedJKT string, key *Key) (*Binding, error) {
	binding, err := NewBinding(token, key)
	if err != nil {
		return nil, err
	}
	if keyID == "" || key.ID() != keyID || expectedJKT == "" || binding.JKT != expectedJKT {
		return nil, errors.New("DPoP key binding does not match stored token metadata")
	}
	return binding, nil
}

func (b *Binding) SignProof(method, rawURL string) (string, error) {
	return b.SignProofContext(context.Background(), method, rawURL)
}

func (b *Binding) SignProofContext(ctx context.Context, method, rawURL string) (string, error) {
	if b == nil || b.key == nil {
		return "", errors.New("DPoP binding key is unavailable")
	}
	return b.key.SignProofContext(ctx, method, rawURL)
}

func (b *Binding) Key() *Key {
	if b == nil {
		return nil
	}
	return b.key
}

type bindingContextKey struct{}
type tokenEndpointKeyContextKey struct{}

func WithBinding(ctx context.Context, binding *Binding) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, bindingContextKey{}, binding)
}

func BindingFromContext(ctx context.Context) (*Binding, bool) {
	if ctx == nil {
		return nil, false
	}
	binding, ok := ctx.Value(bindingContextKey{}).(*Binding)
	return binding, ok && binding != nil
}

// WithTokenEndpointKey marks an OAuth Token Endpoint request for proof
// generation at the network boundary. It carries signing capability only in
// memory and never serializes the private key.
func WithTokenEndpointKey(ctx context.Context, key *Key) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, tokenEndpointKeyContextKey{}, key)
}

func tokenEndpointKeyFromContext(ctx context.Context) (*Key, bool) {
	if ctx == nil {
		return nil, false
	}
	key, ok := ctx.Value(tokenEndpointKeyContextKey{}).(*Key)
	return key, ok && key != nil
}
