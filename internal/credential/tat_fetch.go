// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/dpop"
	"github.com/larksuite/cli/internal/keysigner"
)

type tatResponse struct {
	Code             int    `json:"code"`
	AccessToken      string `json:"access_token"`
	ExpiresIn        int64  `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Msg              string `json:"msg"`
	TokenType        string `json:"token_type"`
}

type FetchedToken struct {
	AccessToken string
	ExpiresIn   int64
	DPoP        *dpop.Binding
}

// FetchTAT mints a tenant token using client_credentials and the supplied DPoP
// mode, serializing issuance per app. Disabled mode does not access key storage,
// so the post-config-init probe can validate credentials without a keychain read.
//
// A deterministic client-side rejection (e.g. invalid_client) returns the
// canonical typed error from classifyTATResponseCode — the SAME classification
// doResolveTAT (and thus every token-resolving command) produces, so callers
// see one consistent envelope. Transport failures, unreadable/unparseable
// bodies, and transient server-side failures (5xx / server_error) are returned
// raw (untyped), leaving them ambiguous. HTTP 429 is the exception: it carries
// typed retry metadata so callers can back off instead of treating it as a
// credential rejection.
//
// The caller owns the context timeout.
func FetchTAT(ctx context.Context, httpClient *http.Client, brand core.LarkBrand, appID, appSecret string, mode core.DPoPMode) (*FetchedToken, error) {
	var result *FetchedToken
	err := larkauth.WithTATIssuanceLock(ctx, appID, func() error {
		var issueErr error
		result, issueErr = fetchTAT(ctx, httpClient, brand, appID, appSecret, mode)
		return issueErr
	})
	return result, err
}

// fetchTAT runs under the issuance lock and reuses a stable per-app key handle.
// The resulting binding stays in memory with the cached TAT.
func fetchTAT(ctx context.Context, httpClient *http.Client, brand core.LarkBrand, appID, appSecret string, mode core.DPoPMode) (result *FetchedToken, retErr error) {
	mode = core.EffectiveDPoPMode(mode)
	var proofKey *dpop.Key
	var keyStore *dpop.KeyStore
	createdKey := false
	keepKey := false
	if mode.Enabled() {
		keyStore = dpop.NewKeyStore(nil)
		if err := keyStore.ProbeWritableContext(ctx); err != nil {
			if mode == core.DPoPModePreferred && errors.Is(err, keysigner.ErrUnavailable) {
				return requestTAT(ctx, httpClient, brand, appID, appSecret, nil, keyStore, false)
			}
			return nil, errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
				"DPoP key storage is unavailable: %v", err).
				WithCause(err).
				WithHint("%s", dpop.KeyStoreUnavailableHint)
		}
		var err error
		proofKey, createdKey, err = keyStore.PrepareReplaceableContext(ctx, tatDPoPKeyID(brand, appID))
		if err != nil {
			if mode == core.DPoPModePreferred && errors.Is(err, keysigner.ErrUnavailable) {
				return requestTAT(ctx, httpClient, brand, appID, appSecret, nil, keyStore, false)
			}
			return nil, errs.NewAuthenticationError(errs.SubtypeDPoPProofFailed,
				"failed to generate DPoP key: %v", err).WithCause(err)
		}
		if createdKey {
			defer func() {
				if !keepKey {
					cleanupCtx := ctx
					if cleanupCtx == nil {
						cleanupCtx = context.Background()
					} else {
						cleanupCtx = context.WithoutCancel(cleanupCtx)
					}
					if cleanupErr := keyStore.DeleteKeyContext(cleanupCtx, proofKey); cleanupErr != nil {
						result = nil
						retErr = errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
							"failed to clean up an uncommitted DPoP key: %v", cleanupErr).
							WithCause(errors.Join(retErr, cleanupErr)).
							WithHint("%s", dpop.KeyStoreUnavailableHint)
					}
				}
			}()
		}
		if err := keyStore.SaveContext(ctx, proofKey); err != nil {
			return nil, errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
				"failed to persist DPoP key reference: %v", err).
				WithCause(err).
				WithHint("%s", dpop.KeyStoreUnavailableHint)
		}
		if err := dpop.SynchronizeClock(ctx, httpClient, brand, proofKey); err != nil {
			if mode == core.DPoPModePreferred && ctx.Err() == nil {
				return requestTAT(ctx, httpClient, brand, appID, appSecret, nil, keyStore, false)
			}
			return nil, err
		}
		if err := keyStore.SaveContext(ctx, proofKey); err != nil {
			return nil, errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
				"failed to persist synchronized DPoP clock: %v", err).
				WithCause(err).
				WithHint("%s", dpop.KeyStoreUnavailableHint)
		}
	}
	result, retErr = requestTAT(ctx, httpClient, brand, appID, appSecret, proofKey, keyStore, false)
	if retErr == nil && result != nil && result.DPoP != nil {
		keepKey = true
	}
	return result, retErr
}

func tatDPoPKeyID(brand core.LarkBrand, appID string) string {
	digest := sha256.Sum256([]byte(string(brand) + "\x00" + appID))
	return "tat-" + base64.RawURLEncoding.EncodeToString(digest[:])
}

func requestTAT(ctx context.Context, httpClient *http.Client, brand core.LarkBrand, appID, appSecret string, proofKey *dpop.Key, keyStore *dpop.KeyStore, clockRetried bool) (*FetchedToken, error) {
	ep := core.ResolveEndpoints(brand)
	endpoint := ep.Accounts + core.OAuthTokenV3Path

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", appID)
	form.Set("client_secret", appSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if proofKey != nil {
		req = req.WithContext(dpop.WithTokenEndpointKey(req.Context(), proofKey))
		proof, proofErr := proofKey.SignProofContext(req.Context(), http.MethodPost, endpoint)
		if proofErr != nil {
			return nil, errs.NewAuthenticationError(errs.SubtypeDPoPProofFailed,
				"failed to generate TAT DPoP proof: %v", proofErr).WithCause(proofErr)
		}
		req.Header.Set(dpop.ProofHeader, proof)
	}

	resp, err := httpClient.Do(req)
	localReceiveTime := time.Now()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read TAT response: %w", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		var rateLimitErr *errs.APIError
		var result tatResponse
		if json.Unmarshal(body, &result) == nil {
			desc := result.ErrorDescription
			if desc == "" {
				desc = result.Msg
			}
			classified := classifyTATResponseCode(result.Code, result.Error, desc, string(brand), appID)
			var apiErr *errs.APIError
			if errors.As(classified, &apiErr) &&
				apiErr.Subtype == errs.SubtypeRateLimit && apiErr.Retryable {
				rateLimitErr = apiErr
			}
		}

		if rateLimitErr == nil {
			rateLimitErr = errs.NewAPIError(errs.SubtypeRateLimit, "TAT endpoint rate limited (HTTP 429)").
				WithCode(http.StatusTooManyRequests).
				WithRetryable()
		}
		if retryAfter := tatRetryAfterSeconds(resp.Header); retryAfter > 0 {
			rateLimitErr.RetryAfterSeconds = retryAfter
			rateLimitErr.Hint = fmt.Sprintf("wait at least %d seconds before retrying; if throttling continues, use exponential backoff with jitter", retryAfter)
		} else {
			rateLimitErr.Hint = "use exponential backoff with jitter when retrying"
		}
		return nil, rateLimitErr
	}

	var result tatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		// An unparseable body is ambiguous (covers non-JSON error pages and
		// truncated payloads); stay untyped so probe callers treat it as noise.
		return nil, fmt.Errorf("failed to parse TAT response (HTTP %d): %w", resp.StatusCode, err)
	}
	if dpop.IsClockRecoverySignal(result.Code, result.Error) && proofKey != nil {
		if clockRetried {
			return nil, errs.NewAuthenticationError(errs.SubtypeDPoPTokenRejected,
				"Token Endpoint rejected DPoP proof after clock recovery").
				WithCode(result.Code).
				WithCause(dpop.ErrInvalidProofResponse).
				WithHint("correct the system clock and retry")
		}
		serverTime, dateErr := http.ParseTime(resp.Header.Get("Date"))
		if dateErr != nil {
			return nil, errs.NewAuthenticationError(errs.SubtypeDPoPClockSyncFailed,
				"Token Endpoint rejected DPoP proof and did not provide a valid server time").
				WithCode(result.Code).WithCause(errors.Join(dpop.ErrInvalidProofResponse, dateErr)).
				WithHint("correct the system clock and retry")
		}
		proofKey.Clock().SetServerTime(serverTime, localReceiveTime)
		if err := keyStore.SaveContext(ctx, proofKey); err != nil {
			return nil, errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
				"failed to persist recovered DPoP clock: %v", err).
				WithCause(err).
				WithHint("%s", dpop.KeyStoreUnavailableHint)
		}
		return requestTAT(ctx, httpClient, brand, appID, appSecret, proofKey, keyStore, true)
	}

	if result.Code == 0 && result.AccessToken != "" {
		if result.ExpiresIn <= 0 {
			return nil, fmt.Errorf("TAT response has invalid expires_in %d", result.ExpiresIn)
		}
		fetched := &FetchedToken{AccessToken: result.AccessToken, ExpiresIn: result.ExpiresIn}
		if proofKey != nil {
			if !strings.EqualFold(result.TokenType, dpop.TokenType) {
				return nil, errs.NewAuthenticationError(errs.SubtypeDPoPRequired,
					"Token Endpoint returned %q token_type for a DPoP request", result.TokenType).
					WithHint("retry after the server supports DPoP; fallback is forbidden after a proof was sent")
			}
			binding, bindErr := dpop.NewBinding(result.AccessToken, proofKey)
			if bindErr != nil {
				return nil, errs.NewAuthenticationError(errs.SubtypeDPoPProofFailed,
					"failed to bind tenant token: %v", bindErr).WithCause(bindErr)
			}
			fetched.DPoP = binding
		} else if strings.EqualFold(result.TokenType, dpop.TokenType) {
			return nil, errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
				"Token Endpoint returned a DPoP token for a request that had no proof key").
				WithHint("enable DPoP and retry so the CLI can bind a key")
		}
		return fetched, nil
	}

	// Transient/server-side failures stay untyped so probe callers stay silent and
	// retryers can back off; only deterministic client rejections are typed. Covers
	// 5xx and the OAuth transient error strings (server_error,
	// temporarily_unavailable, slow_down). HTTP 429 was already returned above
	// as a typed rate-limit error with retry guidance and an upstream delay when available.
	if resp.StatusCode >= 500 ||
		result.Error == "server_error" || result.Error == "temporarily_unavailable" ||
		result.Error == "slow_down" {
		return nil, fmt.Errorf("TAT endpoint transient failure (HTTP %d, code=%d, error=%q): %s",
			resp.StatusCode, result.Code, result.Error, result.ErrorDescription)
	}

	// A 2xx with neither token nor error is a malformed success — ambiguous, untyped.
	if result.Code == 0 && result.Error == "" {
		return nil, fmt.Errorf("TAT response missing access_token (HTTP %d)", resp.StatusCode)
	}

	// Prefer the OAuth error_description; fall back to the legacy Lark `msg` so a
	// gateway-level {code, msg} response (carrying no OAuth fields) still yields a
	// non-empty typed message instead of a bare "API error: [code]".
	desc := result.ErrorDescription
	if desc == "" {
		desc = result.Msg
	}
	return nil, classifyTATResponseCode(result.Code, result.Error, desc, string(brand), appID)
}

func tatRetryAfterSeconds(header http.Header) int {
	for _, name := range []string{"X-Ogw-Ratelimit-Reset", "Retry-After"} {
		seconds, err := strconv.Atoi(strings.TrimSpace(header.Get(name)))
		if err == nil && seconds > 0 {
			return seconds
		}
	}
	return 0
}
