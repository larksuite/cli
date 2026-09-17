// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
)

// agentEmployeeUserType is the user_type the Introspection endpoint reports for
// an Agent Employee. It is the open platform's value, not one the CLI chooses:
// tenant and Agent tokens come off the same client_credentials call, and this
// field is the only thing in the response that tells them apart.
const agentEmployeeUserType int64 = 66

type introspectionResponse struct {
	Active           bool   `json:"active"`
	ClientID         string `json:"client_id"`
	TokenType        string `json:"token_type"`
	IssuedAt         int64  `json:"iat"`
	ExpiresAt        int64  `json:"exp"`
	UserType         int64  `json:"user_type"`
	OpenID           string `json:"open_id"`
	Code             int    `json:"code"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Msg              string `json:"msg"`
}

type TokenIntrospection struct {
	ClientID  string
	TokenType string
	IssuedAt  time.Time
	ExpiresAt time.Time
	UserType  int64
	OpenID    string
}

func (i *TokenIntrospection) IsAgentEmployee() bool {
	return i != nil && i.UserType == agentEmployeeUserType
}

func IntrospectAccessToken(
	ctx context.Context,
	httpClient *http.Client,
	brand core.LarkBrand,
	appID, appSecret, token string,
) (*TokenIntrospection, error) {
	if httpClient == nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError,
			"HTTP client is unavailable for token Introspection")
	}
	endpoint := core.ResolveEndpoints(brand).Accounts + auth.PathOAuthIntrospect
	form := url.Values{}
	form.Set("token", token)
	form.Set("client_id", appID)
	form.Set("client_secret", appSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError,
			"failed to create token Introspection request").WithCause(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		subtype := errs.SubtypeNetworkTransport
		if errors.Is(err, context.DeadlineExceeded) {
			subtype = errs.SubtypeNetworkTimeout
		}
		typed := errs.NewNetworkError(subtype, "token Introspection request failed").WithCause(err)
		if !errors.Is(err, context.Canceled) {
			typed.WithRetryable()
		}
		return nil, typed
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"failed to read token Introspection response").WithCause(err)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		rateErr := errs.NewAPIError(errs.SubtypeRateLimit,
			"token Introspection endpoint rate limited").
			WithCode(http.StatusTooManyRequests).
			WithRetryable()
		if retryAfter := tatRetryAfterSeconds(resp.Header); retryAfter > 0 {
			rateErr.RetryAfterSeconds = retryAfter
			rateErr.Hint = fmt.Sprintf("wait at least %d seconds before retrying", retryAfter)
		} else {
			rateErr.Hint = "use exponential backoff with jitter when retrying"
		}
		return nil, rateErr
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkServer,
			"token Introspection endpoint failed with HTTP %d", resp.StatusCode).
			WithCode(resp.StatusCode).
			WithRetryable()
	}

	var result introspectionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"failed to decode token Introspection response").WithCause(err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices ||
		result.Code != 0 || result.Error != "" {
		return nil, introspectionResponseError(result)
	}

	now := time.Now()
	expiresAt := time.Unix(result.ExpiresAt, 0)
	issuedAt := time.Unix(result.IssuedAt, 0)
	if !result.Active {
		return nil, errs.NewAuthenticationError(errs.SubtypeTokenInvalid,
			"OAuth token is not active").
			WithHint("Check that the Agent Employee is enabled and rerun `lark-cli config init`.")
	}
	if result.ClientID != appID {
		return nil, errs.NewConfigError(errs.SubtypeInvalidClient,
			"token Introspection returned a different client ID").
			WithHint("Check the app ID and app secret, then rerun `lark-cli config init`.")
	}
	if result.ExpiresAt <= 0 || !expiresAt.After(now) {
		return nil, errs.NewAuthenticationError(errs.SubtypeTokenExpired,
			"Introspection returned an expired token")
	}
	if result.IssuedAt > 0 && result.ExpiresAt <= result.IssuedAt {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"token Introspection returned an invalid lifetime")
	}
	if result.UserType == agentEmployeeUserType && result.OpenID == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"Agent Employee Introspection response has no open ID")
	}

	introspection := &TokenIntrospection{
		ClientID:  result.ClientID,
		TokenType: result.TokenType,
		ExpiresAt: expiresAt,
		IssuedAt:  issuedAt,
		UserType:  result.UserType,
		OpenID:    result.OpenID,
	}
	return introspection, nil
}

func introspectionResponseError(result introspectionResponse) error {
	message := result.ErrorDescription
	if message == "" {
		message = result.Error
	}
	if message == "" {
		message = "unknown Introspection failure"
	}

	switch result.Error {
	case "invalid_client", "unauthorized_client":
		return errs.NewConfigError(errs.SubtypeInvalidClient,
			"token Introspection rejected the app credentials: %s", message).
			WithCode(result.Code).
			WithHint("Check the app ID and app secret, then rerun `lark-cli config init`.")
	case "invalid_token":
		return errs.NewAuthenticationError(errs.SubtypeTokenInvalid,
			"token Introspection rejected the access token: %s", message).
			WithCode(result.Code).
			WithHint("Check that the Agent Employee is enabled and rerun `lark-cli config init`.")
	default:
		return errs.NewAPIError(errs.SubtypeUnknown,
			"token Introspection failed: %s", message).
			WithCode(result.Code)
	}
}

func InspectClientCredentials(
	ctx context.Context,
	httpClient *http.Client,
	brand core.LarkBrand,
	appID, appSecret string,
) (*ClientCredentialsToken, *TokenIntrospection, error) {
	minted, err := FetchClientCredentialsToken(ctx, httpClient, brand, appID, appSecret)
	if err != nil {
		if errs.IsTyped(err) {
			return nil, nil, err
		}
		subtype := errs.SubtypeNetworkTransport
		if errors.Is(err, context.DeadlineExceeded) {
			subtype = errs.SubtypeNetworkTimeout
		}
		typed := errs.NewNetworkError(subtype,
			"client_credentials token request failed").
			WithCause(err)
		if !errors.Is(err, context.Canceled) {
			typed.WithRetryable()
		}
		return nil, nil, typed
	}
	if minted.ExpiresAt.IsZero() || !minted.ExpiresAt.After(minted.GrantedAt) {
		return nil, nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"client_credentials response has no valid expiry")
	}
	introspection, err := IntrospectAccessToken(ctx, httpClient, brand, appID, appSecret, minted.Token)
	if err != nil {
		return nil, nil, err
	}
	return minted, introspection, nil
}
