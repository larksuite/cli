// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/dpop"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
)

type failFirstTATAssertionSigner struct {
	keysigner.Signer
	err   error
	calls int
}

func (s *failFirstTATAssertionSigner) Sign(ctx context.Context, ref keysigner.KeyRef, input []byte) ([]byte, string, error) {
	s.calls++
	if s.calls == 1 {
		return nil, "", s.err
	}
	return s.Signer.Sign(ctx, ref, input)
}

func TestFetchTATApplicationSigningFailureDoesNotFallBack(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		deleteErr error
	}{
		{name: "temporary", err: errors.New("temporary application signer failure")},
		{name: "unavailable", err: keysigner.ErrUnavailable},
		{name: "DPoP subtype", err: errs.NewAuthenticationError(errs.SubtypeDPoPProofFailed, "application signer failed").WithCause(keysigner.ErrUnavailable)},
		{name: "repeated proof sentinel", err: dpop.ErrRepeatedInvalidProof},
		{name: "canceled", err: context.Canceled},
		{name: "deadline", err: context.DeadlineExceeded},
		{name: "policy", err: errs.NewSecurityPolicyError(errs.SubtypeAccessDenied, "application signing denied")},
		{name: "cleanup failure", err: keysigner.ErrUnavailable, deleteErr: errors.New("cleanup denied")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := &failFirstTATAssertionSigner{Signer: newFakeTATSigner(t), err: tc.err}
			proof := newTATTestSigner()
			store := dpop.NewKeyStoreWithSigner(tatDPoPMetadata{}, proof)
			heartbeats, tokenCalls := 0, 0
			client := &http.Client{Transport: tatRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				body := fmt.Sprintf(`{"code":0,"data":{"now":"%d"}}`, time.Now().Unix())
				if req.URL.Path == dpop.HeartbeatPath {
					heartbeats++
					// Fail only uncommitted-key cleanup, not the earlier probe.
					proof.deleteErr = tc.deleteErr
				} else {
					tokenCalls++
					body = `{"access_token":"must-not-use","expires_in":120,"token_type":"Bearer"}`
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})}
			ca := auth.ClientAuth{AppID: "cli-tat-sign-failure", AuthMethod: core.AuthMethodPrivateKeyJWT, Signer: app, KeyLabel: "app-key"}
			token, err := fetchTAT(context.Background(), client, core.BrandFeishu, ca, core.DPoPModePreferred, store)
			if token != nil || !errors.Is(err, tc.err) || app.calls != 1 || tokenCalls != 0 || heartbeats != 1 {
				t.Fatalf("application failure = (%+v, %v), assertion signs=%d, token calls=%d, heartbeats=%d; want original failure, one sign, zero token calls, one heartbeat", token, err, app.calls, tokenCalls, heartbeats)
			}
			if tc.deleteErr != nil {
				problem, ok := errs.ProblemOf(err)
				if !ok || problem.Category != errs.CategoryAuthentication || problem.Subtype != errs.SubtypeDPoPKeyMissing || !errors.Is(err, tc.deleteErr) {
					t.Fatalf("cleanup classification/cause changed: %v", err)
				}
			} else {
				if want, ok := errs.ProblemOf(tc.err); ok {
					got, ok := errs.ProblemOf(err)
					if !ok || got.Category != want.Category || got.Subtype != want.Subtype {
						t.Fatalf("application error classification changed: %v", err)
					}
				}
				if len(proof.keys) != 0 {
					t.Fatalf("uncommitted proof keys retained: %d", len(proof.keys))
				}
			}
		})
	}
}

func TestFetchTATPrivateKeyJWTWithDPoP(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     core.DPoPMode
		fallback bool
	}{
		{"required", core.DPoPModeRequired, false},
		{"preferred fallback", core.DPoPModePreferred, true},
		{"disabled", core.DPoPModeDisabled, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appSigner := newFakeTATSigner(t)
			kid, err := keysigner.PublicKeyThumbprint(appSigner.key.Public())
			if err != nil {
				t.Fatal(err)
			}
			store := dpop.NewKeyStoreWithSigner(tatDPoPMetadata{}, newTATTestSigner())
			ca := auth.ClientAuth{AppID: "cli-tat-pkjwt", AppSecret: "must-not-send", AuthMethod: core.AuthMethodPrivateKeyJWT, Signer: appSigner, KeyLabel: "app-key"}
			calls := 0
			seen := map[string]bool{}
			client := &http.Client{Transport: tatRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				body := `{"code":0,"data":{"now":"` + strconv.FormatInt(time.Now().Unix(), 10) + `"}}`
				if req.URL.Path != dpop.HeartbeatPath {
					calls++
					if req.URL.String() != core.ResolveEndpoints(core.BrandFeishu).Open+auth.PathOAuthTokenV2 {
						t.Fatalf("PKJWT TAT endpoint changed: %s", req.URL)
					}
					raw, err := io.ReadAll(req.Body)
					if err != nil {
						t.Fatal(err)
					}
					form, err := url.ParseQuery(string(raw))
					if err != nil {
						t.Fatal(err)
					}
					if form.Has("client_secret") || req.Header.Get("Authorization") != "" || form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
						t.Fatal("PKJWT TAT wire contract changed")
					}
					parts := strings.Split(form.Get("client_assertion"), ".")
					if len(parts) != 3 {
						t.Fatal("missing assertion")
					}
					h, err := base64.RawURLEncoding.DecodeString(parts[0])
					if err != nil {
						t.Fatal(err)
					}
					var header map[string]any
					if err := json.Unmarshal(h, &header); err != nil {
						t.Fatal(err)
					}
					if header["kid"] != kid || header["typ"] != "JWT" {
						t.Fatal("wrong application key")
					}
					for _, jwt := range []string{form.Get("client_assertion"), req.Header.Get(dpop.ProofHeader)} {
						if jwt == "" {
							continue
						}
						if seen[jwt] {
							t.Fatal("JWT was reused on retry")
						}
						seen[jwt] = true
					}
					wantProof := tc.mode != core.DPoPModeDisabled && (!tc.fallback || calls <= 3)
					if (req.Header.Get(dpop.ProofHeader) != "") != wantProof {
						t.Fatal("incorrect DPoP policy")
					}
					if tc.fallback && calls <= 3 {
						body = `{"error":"` + dpop.InvalidProofOAuthError + `"}`
					} else {
						tokenType := "Bearer"
						if wantProof {
							tokenType = dpop.TokenType
						}
						body = `{"tenant_access_token":"tenant-token","expires_in":120,"token_type":"` + tokenType + `"}`
					}
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})}
			token, err := fetchTAT(context.Background(), client, core.BrandFeishu, ca, tc.mode, store)
			if err != nil || token == nil || token.AccessToken != "tenant-token" || token.ExpiresIn != 120 {
				t.Fatalf("TAT = (%+v, %v)", token, err)
			}
			wantCalls := 1
			if tc.fallback {
				wantCalls = 4
			}
			if calls != wantCalls {
				t.Fatalf("calls=%d, want %d", calls, wantCalls)
			}
			if tc.mode == core.DPoPModeRequired {
				if token.DPoP == nil {
					t.Fatal("required TAT lost binding")
				}
				jkt, err := token.DPoP.Key().Thumbprint()
				if err != nil || kid == jkt {
					t.Fatal("application key reused for token binding")
				}
			} else if token.DPoP != nil {
				t.Fatal("Bearer fallback kept DPoP binding")
			}
		})
	}
}

func TestDefaultTokenProviderPrivateKeyJWTCachesActualExpiry(t *testing.T) {
	signer := newFakeTATSigner(t)
	config := &core.MultiAppConfig{Apps: []core.AppConfig{{AppId: "cli-pkjwt-cache", Brand: core.BrandFeishu, AuthMethod: core.AuthMethodPrivateKeyJWT,
		KeyRef: &core.SecretRef{Source: core.SecretSourceTEE, ID: "app-key", Provider: keysigner.MacOSKeychainSignerName}, DPoPMode: core.DPoPModeDisabled}}}
	if err := core.SaveMultiAppConfig(config); err != nil {
		t.Fatal(err)
	}
	account := NewDefaultAccountProvider(func() keychain.KeychainAccess { return &tenantTokenStoreKeychain{} }, "", core.ProfileFromConfig)
	calls := 0
	client := &http.Client{Transport: tatRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"access_token":"tat-%d","expires_in":120,"token_type":"Bearer"}`, calls))), Request: req}, nil
	})}
	provider := NewDefaultTokenProvider(account, func() (*http.Client, error) { return client, nil }, io.Discard, func(cfg *core.CliConfig) (keysigner.Signer, error) {
		if cfg.AuthMethod != core.AuthMethodPrivateKeyJWT || cfg.KeyLabel != "app-key" || cfg.DPoPMode != core.DPoPModeDisabled || cfg.CredentialSource != core.CredentialSourceLocal {
			t.Fatal("configuration lost authentication or DPoP fields")
		}
		return signer, nil
	})
	now := time.Now()
	provider.timeNow = func() time.Time { return now }
	for _, want := range []string{"tat-1", "tat-1", "tat-2"} {
		if want == "tat-2" {
			now = now.Add(121 * time.Second)
		}
		got, err := provider.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeTAT})
		if err != nil || got == nil || got.Token != want {
			t.Fatalf("cached TAT = (%+v,%v), want %s", got, err, want)
		}
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want 2", calls)
	}
}
