// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package identitydiag

import (
	"context"
	"net/http"
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestExtendedDiagnosticsVerifyOnDemandUserThroughDataPlane(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := &core.CliConfig{
		AppID: "cli_x", Brand: core.BrandFeishu,
		SupportedIdentities: uint8(extcred.SupportsUser),
	}
	f, _, _, registry := cmdutil.TestFactory(t, cfg)
	f.Credential = credential.NewCredentialProvider([]extcred.Provider{&fakeExtProvider{
		name:    "managed-process",
		account: &extcred.Account{AppID: "cli_x", SupportedIdentities: extcred.SupportsUser},
		token:   &extcred.Token{Value: "ext-uat"},
		caps: credential.ProviderCapabilities{
			SkipUserInfoEnrichment: true,
			ProvidesOnDemandAuth:   true,
		},
	}}, nil, nil, f.HttpClient)
	stub := &httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/authen/v1/user_info",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"open_id": "ou_external", "name": "External User"},
		},
	}
	registry.Register(stub)

	got := Diagnose(context.Background(), f, cfg, true)
	if !got.User.Available || got.User.Verified == nil || !*got.User.Verified {
		t.Fatalf("user = %#v, want verified managed user", got.User)
	}
	if got.User.OpenID != "ou_external" || got.User.UserName != "External User" {
		t.Fatalf("user identity = %#v", got.User)
	}
	if header := stub.CapturedHeaders.Get("Authorization"); header != "Bearer ext-uat" {
		t.Fatalf("Authorization = %q", header)
	}
}

func TestExtendedDiagnosticsRejectEmptyOnDemandIdentity(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := &core.CliConfig{
		AppID: "cli_x", Brand: core.BrandFeishu,
		SupportedIdentities: uint8(extcred.SupportsUser),
	}
	f, _, _, registry := cmdutil.TestFactory(t, cfg)
	f.Credential = credential.NewCredentialProvider([]extcred.Provider{&fakeExtProvider{
		name:    "managed-process",
		account: &extcred.Account{AppID: "cli_x", SupportedIdentities: extcred.SupportsUser},
		token:   &extcred.Token{Value: "ext-uat"},
		caps: credential.ProviderCapabilities{
			SkipUserInfoEnrichment: true,
			ProvidesOnDemandAuth:   true,
		},
	}}, nil, nil, f.HttpClient)
	registry.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/authen/v1/user_info",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"name": "Missing ID"},
		},
	})

	got := Diagnose(context.Background(), f, cfg, true)
	if got.User.Available ||
		got.User.Verified == nil ||
		*got.User.Verified ||
		got.User.Status != StatusVerifyFailed {
		t.Fatalf("user = %#v, want verify_failed/unavailable", got.User)
	}
}

func TestFetchEditionUserInfoPreservesHTTPStatusForInvalidBody(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := &core.CliConfig{AppID: "cli_x", Brand: core.BrandFeishu}
	f, _, _, registry := cmdutil.TestFactory(t, cfg)
	registry.Register(&httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "/open-apis/authen/v1/user_info",
		Status:  http.StatusBadGateway,
		RawBody: []byte("<html>bad gateway</html>"),
	})

	_, err := fetchEditionUserInfo(context.Background(), f, cfg, "external-token")
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("error = %v, want HTTP 502", err)
	}
}

func TestFetchEditionUserInfoPreservesStructuredHTTPError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := &core.CliConfig{AppID: "cli_x", Brand: core.BrandFeishu}
	f, _, _, registry := cmdutil.TestFactory(t, cfg)
	registry.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/authen/v1/user_info",
		Status: http.StatusForbidden,
		Body:   map[string]interface{}{"code": 20029, "msg": "permission denied"},
	})

	_, err := fetchEditionUserInfo(context.Background(), f, cfg, "external-token")
	if err == nil || !strings.Contains(err.Error(), "HTTP 403, code 20029: permission denied") {
		t.Fatalf("error = %v, want structured HTTP error", err)
	}
}
