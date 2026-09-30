// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/dpop"
	"github.com/larksuite/cli/internal/keysigner"
)

func jwtTestPart(t *testing.T, value string, index int) map[string]any {
	t.Helper()
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		t.Fatal("invalid compact JWT")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[index])
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func checkPrivateKeyJWTRequest(t *testing.T, assertion, proof, appID, kid string, seen map[string]bool) {
	t.Helper()
	header, claims := jwtTestPart(t, assertion, 0), jwtTestPart(t, assertion, 1)
	if header["typ"] != "JWT" || header["kid"] != kid || claims["iss"] != appID || claims["sub"] != appID || claims["aud"] != core.ClientAssertionAudience(core.BrandFeishu) {
		t.Fatalf("unexpected assertion identity: header=%v claims=%v", header, claims)
	}
	jti, _ := claims["jti"].(string)
	if jti == "" || seen[jti] {
		t.Fatal("client assertion was reused")
	}
	seen[jti] = true
	if proof != "" {
		proofHeader := jwtTestPart(t, proof, 0)
		if proofHeader["typ"] != "dpop+jwt" {
			t.Fatal("assertion used as DPoP proof")
		}
		proofClaims := jwtTestPart(t, proof, 1)
		proofJTI, _ := proofClaims["jti"].(string)
		if proofJTI == "" || seen[proofJTI] {
			t.Fatal("DPoP proof was reused")
		}
		seen[proofJTI] = true
	}
}

func TestDeviceFlowPrivateKeyJWTWithDPoP(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     core.DPoPMode
		fallback bool
	}{
		{"disabled", core.DPoPModeDisabled, false},
		{"required", core.DPoPModeRequired, false},
		{"preferred proof rejection", core.DPoPModePreferred, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appSigner := newFakeAuthSigner(t)
			kid, err := keysigner.PublicKeyThumbprint(appSigner.key.Public())
			if err != nil {
				t.Fatal(err)
			}
			proofSigner := newAuthDPoPTestSigner()
			store := dpop.NewKeyStoreWithSigner(deviceFlowMetadata{}, proofSigner)
			ca := ClientAuth{AppID: "cli-pkjwt", AppSecret: "must-not-send", AuthMethod: core.AuthMethodPrivateKeyJWT, Signer: appSigner, KeyLabel: "app-key"}
			seen, calls := map[string]bool{}, 0
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == dpop.HeartbeatPath {
					return refreshHTTPResponse(req, `{"code":0,"data":{"now":"`+strconv.FormatInt(time.Now().Unix(), 10)+`"}}`), nil
				}
				calls++
				if req.URL.String() != ResolveOAuthEndpoints(core.BrandFeishu).Token {
					t.Fatalf("unexpected endpoint: %s", req.URL)
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				form, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatal(err)
				}
				if form.Has("client_secret") || req.Header.Get("Authorization") != "" {
					t.Fatal("PKJWT leaked secret authentication")
				}
				proof := req.Header.Get(dpop.ProofHeader)
				checkPrivateKeyJWTRequest(t, form.Get("client_assertion"), proof, ca.AppID, kid, seen)
				if tc.fallback && calls <= 3 {
					if proof == "" {
						t.Fatal("preferred issuance skipped proof before rejection")
					}
					return refreshHTTPResponse(req, `{"error":"`+dpop.InvalidProofOAuthError+`"}`), nil
				}
				wantProof := tc.mode != core.DPoPModeDisabled && !tc.fallback
				if (proof != "") != wantProof {
					t.Fatal("incorrect DPoP fallback policy")
				}
				if wantProof && calls == 1 {
					return refreshHTTPResponse(req, `{"error":"authorization_pending"}`), nil
				}
				tokenType := "Bearer"
				if wantProof {
					tokenType = dpop.TokenType
				}
				return refreshHTTPResponse(req, `{"access_token":"access","refresh_token":"refresh","token_type":"`+tokenType+`","expires_in":7200,"refresh_token_expires_in":86400}`), nil
			})}
			result, err := pollDeviceTokenWithKeyStore(context.Background(), client, ca, core.BrandFeishu, "device-code", 1, 15, io.Discard, tc.mode, store)
			if err != nil || result == nil || !result.OK {
				t.Fatalf("device flow = (%+v, %v)", result, err)
			}
			if result.Token.DPoP != nil {
				jkt, err := result.Token.DPoP.Key().Thumbprint()
				if err != nil || jkt == kid {
					t.Fatalf("application key reused as proof key: %v", err)
				}
			} else if tc.mode == core.DPoPModeRequired {
				t.Fatal("required token binding lost")
			}
			wantCalls := 1
			if tc.fallback {
				wantCalls = 4
			} else if tc.mode == core.DPoPModeRequired {
				wantCalls = 2
			}
			if calls != wantCalls {
				t.Fatalf("token calls=%d, want %d", calls, wantCalls)
			}
		})
	}
}

func TestRefreshPrivateKeyJWTWithDPoPPreservesBinding(t *testing.T) {
	appSigner := newFakeAuthSigner(t)
	setupStoredTokenTest(t)
	store, key := newAuthDPoPStore(t)
	if err := store.SaveContext(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	jkt, err := key.Thumbprint()
	if err != nil {
		t.Fatal(err)
	}
	kid, err := keysigner.PublicKeyThumbprint(appSigner.key.Public())
	if err != nil {
		t.Fatal(err)
	}
	if kid == jkt {
		t.Fatal("keys are not independent")
	}
	stored := newRefreshTestToken()
	stored.TokenType, stored.DPoPKeyID, stored.DPoPJKT, stored.DPoPKeySecurityLevel = StoredTokenTypeDPoP, key.ID(), jkt, string(key.SecurityLevel())
	if err := SetStoredToken(stored); err != nil {
		t.Fatal(err)
	}
	opts := newRefreshTestOptions(stored)
	opts.AuthMethod, opts.Signer, opts.KeyLabel = core.AuthMethodPrivateKeyJWT, appSigner, "app-key"
	opts.DPoPMode, opts.DPoPKeyStore = core.DPoPModeRequired, store
	seen, calls := map[string]bool{}, 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == dpop.HeartbeatPath {
			return refreshHTTPResponse(req, `{"code":0,"data":{"now":"`+strconv.FormatInt(time.Now().Unix(), 10)+`"}}`), nil
		}
		calls++
		var payload map[string]string
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if _, ok := payload["client_secret"]; ok {
			t.Fatal("refresh sent client_secret")
		}
		proof := req.Header.Get(dpop.ProofHeader)
		if proof == "" {
			t.Fatal("refresh lost DPoP proof")
		}
		checkPrivateKeyJWTRequest(t, payload["client_assertion"], proof, opts.AppId, kid, seen)
		if calls == 1 {
			return refreshHTTPResponse(req, `{"code":20050,"error_description":"retry"}`), nil
		}
		return refreshHTTPResponse(req, `{"access_token":"access-new","refresh_token":"refresh-new","token_type":"DPoP","expires_in":7200,"refresh_token_expires_in":86400}`), nil
	})}
	result, err := GetValidAccessToken(context.Background(), client, opts)
	if err != nil || result == nil || result.DPoP == nil || calls != 2 {
		t.Fatalf("refresh = (%+v, %v), calls=%d", result, err, calls)
	}
	current := mustGetStoredToken(t, stored.AppId, stored.UserOpenId)
	if current == nil || current.DPoPKeyID != key.ID() || current.DPoPJKT != jkt || current.RefreshToken != "refresh-new" {
		t.Fatal("refresh lost binding or rotation")
	}
}

func TestDeviceFlowPrivateKeyJWTSigningFailureDoesNotDowngrade(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	signer := newAuthDPoPTestSigner()
	store := dpop.NewKeyStoreWithSigner(deviceFlowMetadata{}, signer)
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != dpop.HeartbeatPath {
			calls++
			t.Error("signing failure sent token request")
		}
		return refreshHTTPResponse(req, `{}`), nil
	})}
	result, err := pollDeviceTokenWithKeyStore(context.Background(), client, ClientAuth{AppID: "cli", AppSecret: "must-not-send", AuthMethod: core.AuthMethodPrivateKeyJWT}, core.BrandFeishu, "device", 1, 3, io.Discard, core.DPoPModePreferred, store)
	if result != nil || err == nil || calls != 0 {
		t.Fatalf("signing failure = (%+v, %v)", result, err)
	}
	if len(signer.keys) != 0 {
		t.Fatal("signing failure retained an uncommitted DPoP key")
	}
}
