// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
)

const testAgentToken = "agent-employee-token"

// agentProvider is an external credential source for one Agent Employee. It
// exists so these tests exercise the real resolution pipeline rather than
// injecting a credential past the checks that guard it.
type agentProvider struct {
	appID     string
	principal string
	kind      extcred.CredentialKind
}

func (p *agentProvider) Name() string { return "test-agent" }

func (p *agentProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return &extcred.Account{
		AppID:               p.appID,
		AppSecret:           extcred.NoAppSecret,
		Brand:               extcred.BrandFeishu,
		OpenID:              p.principal,
		CredentialKind:      extcred.CredentialAAT,
		SupportedIdentities: extcred.SupportsUser,
	}, nil
}

func (p *agentProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return nil, errors.New("an Agent Employee must not be resolved through the legacy token path")
}

func (p *agentProvider) ResolveAccess(_ context.Context, req extcred.AccessSpec) (*extcred.Access, error) {
	kind := p.kind
	if kind == "" {
		kind = extcred.CredentialAAT
	}
	return &extcred.Access{
		Value:     testAgentToken,
		Kind:      kind,
		OpenID:    p.principal,
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil
}

func newAgentAPIClient(t *testing.T, rt http.RoundTripper) *APIClient {
	t.Helper()
	httpClient := &http.Client{Transport: rt}
	sdk := lark.NewClient("cli_agent", "test-secret",
		lark.WithEnableTokenCache(false),
		lark.WithLogLevel(larkcore.LogLevelError),
		lark.WithHttpClient(httpClient),
	)
	cred := credential.NewCredentialProvider(
		[]extcred.Provider{&agentProvider{appID: "cli_agent", principal: "ou_agent"}},
		nil, nil, nil,
	)
	return &APIClient{
		SDK:        sdk,
		HTTP:       httpClient,
		ErrOut:     &bytes.Buffer{},
		Credential: cred,
		Config: &core.CliConfig{
			AppID:           "cli_agent",
			Brand:           core.BrandFeishu,
			CredentialKind:  core.CredentialAAT,
			PrincipalOpenID: "ou_agent",
			UserOpenId:      "ou_agent",
		},
	}
}

// TestDoSDKRequestSendsAgentTokenOnUserLane pins that an Agent Employee
// credential travels as a user access token. Sending it as a tenant token would
// still return HTTP 200 while attributing the call to the application instead of
// the Agent, which is the failure mode hardest to notice after the fact.
func TestDoSDKRequestSendsAgentTokenOnUserLane(t *testing.T) {
	var gotAuth string
	c := newAgentAPIClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotAuth = req.Header.Get("Authorization")
		return jsonResponse(map[string]interface{}{"code": 0, "data": map[string]interface{}{}}), nil
	}))

	req := &larkcore.ApiReq{HttpMethod: http.MethodGet, ApiPath: "/open-apis/im/v1/chats"}
	if _, err := c.DoSDKRequest(context.Background(), req, core.AsUser); err != nil {
		t.Fatalf("DoSDKRequest() error = %v", err)
	}
	if gotAuth != "Bearer "+testAgentToken {
		t.Fatalf("Authorization = %q, want the Agent Employee token", gotAuth)
	}
	if len(req.SupportedAccessTokenTypes) != 1 ||
		req.SupportedAccessTokenTypes[0] != larkcore.AccessTokenTypeUser {
		t.Fatalf("SupportedAccessTokenTypes = %v, want user only", req.SupportedAccessTokenTypes)
	}
}

// TestDoSDKRequestRefusesAgentTokenAsBot pins that the refusal happens before
// any request goes out, so no attempt is made to mint or send a tenant token for
// an Agent Employee profile.
func TestDoSDKRequestRefusesAgentTokenAsBot(t *testing.T) {
	requests := 0
	c := newAgentAPIClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		return jsonResponse(map[string]interface{}{"code": 0}), nil
	}))

	req := &larkcore.ApiReq{HttpMethod: http.MethodGet, ApiPath: "/open-apis/im/v1/chats"}
	_, err := c.DoSDKRequest(context.Background(), req, core.AsBot)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("error = %T %v, want failed_precondition", err, err)
	}
	if requests != 0 {
		t.Fatalf("%d requests were sent; the refusal must happen before the network", requests)
	}
}

// TestDoStreamSendsAgentToken pins that the streaming path resolves its own
// credential through the same entry point. It builds its Authorization header by
// hand, so it cannot inherit the fix applied to the SDK path.
func TestDoStreamSendsAgentToken(t *testing.T) {
	var gotAuth string
	c := newAgentAPIClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotAuth = req.Header.Get("Authorization")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/octet-stream"}},
			Body:       http.NoBody,
		}, nil
	}))

	resp, err := c.DoStream(context.Background(), &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    "/open-apis/drive/v1/files/token/download",
	}, core.AsUser)
	if err != nil {
		t.Fatalf("DoStream() error = %v", err)
	}
	defer resp.Body.Close()
	if gotAuth != "Bearer "+testAgentToken {
		t.Fatalf("Authorization = %q, want the Agent Employee token", gotAuth)
	}
}

// TestMissingAgentCredentialDoesNotSuggestHumanLogin pins that an Agent
// Employee profile is never told to complete a login flow: there is no person
// behind the credential to complete one.
func TestMissingAgentCredentialDoesNotSuggestHumanLogin(t *testing.T) {
	err := newTokenMissingError(core.AsUser, core.CredentialAAT, errors.New("cache empty"))
	var authErr *errs.AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("error = %T, want *errs.AuthenticationError", err)
	}
	if authErr.Hint == "" {
		t.Fatal("an Agent Employee failure still needs actionable recovery")
	}
	if bytes.Contains([]byte(authErr.Hint), []byte("auth login")) {
		t.Fatalf("hint must not point at a human login flow, got %q", authErr.Hint)
	}
}
