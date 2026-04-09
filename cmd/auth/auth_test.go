// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/registry"
)

func TestAuthLoginCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *LoginOptions
	cmd := NewCmdAuthLogin(f, func(opts *LoginOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"--scope", "calendar:calendar:read", "--json"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Scope != "calendar:calendar:read" {
		t.Errorf("expected scope calendar:calendar:read, got %s", gotOpts.Scope)
	}
	if !gotOpts.JSON {
		t.Error("expected JSON=true")
	}
}

func TestAuthCheckCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *CheckOptions
	cmd := NewCmdAuthCheck(f, func(opts *CheckOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"--scope", "calendar:calendar:read drive:drive:read"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Scope != "calendar:calendar:read drive:drive:read" {
		t.Errorf("expected scope string, got %s", gotOpts.Scope)
	}
}

func TestAuthLogoutCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *LogoutOptions
	cmd := NewCmdAuthLogout(f, func(opts *LogoutOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts == nil {
		t.Error("expected opts to be set")
	}
}

func TestAuthListCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *ListOptions
	cmd := NewCmdAuthList(f, func(opts *ListOptions) error {
		gotOpts = opts
		return nil
	})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts == nil {
		t.Error("expected opts to be set")
	}
}

func TestAuthStatusCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *StatusOptions
	cmd := NewCmdAuthStatus(f, func(opts *StatusOptions) error {
		gotOpts = opts
		return nil
	})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts == nil {
		t.Error("expected opts to be set")
	}
}

func TestAuthStatusCmd_VerifyFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *StatusOptions
	cmd := NewCmdAuthStatus(f, func(opts *StatusOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"--verify"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts == nil {
		t.Fatal("expected opts to be set")
	}
	if !gotOpts.Verify {
		t.Error("expected Verify=true when --verify flag is passed")
	}
}

func TestDomainFlagCompletion(t *testing.T) {
	allDomains := registry.ListFromMetaProjects()

	tests := []struct {
		name         string
		toComplete   string
		wantContains []string
		wantExclude  []string
	}{
		{
			name:         "empty returns all domains",
			toComplete:   "",
			wantContains: allDomains,
		},
		{
			name:         "partial match",
			toComplete:   "cal",
			wantContains: []string{"calendar"},
			wantExclude:  []string{"bitable", "drive", "task"},
		},
		{
			name:       "comma prefix completes second value",
			toComplete: "calendar,",
			wantContains: func() []string {
				var out []string
				for _, d := range allDomains {
					out = append(out, "calendar,"+d)
				}
				return out
			}(),
		},
		{
			name:         "comma with partial second value",
			toComplete:   "calendar,ta",
			wantContains: []string{"calendar,task"},
			wantExclude:  []string{"calendar,bitable", "calendar,drive"},
		},
		{
			name:       "no match returns empty",
			toComplete: "xxx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comps := completeDomain(tt.toComplete)
			sort.Strings(comps)

			for _, want := range tt.wantContains {
				found := false
				for _, c := range comps {
					if c == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("completions %v missing expected %q", comps, want)
				}
			}

			for _, exclude := range tt.wantExclude {
				for _, c := range comps {
					if c == exclude {
						t.Errorf("completions %v should not contain %q", comps, exclude)
					}
				}
			}

			// Verify no completion contains trailing comma artifacts
			for _, c := range comps {
				if strings.HasSuffix(c, ",") {
					t.Errorf("completion %q should not end with comma", c)
				}
			}
		})
	}
}

func TestAuthScopesCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *ScopesOptions
	cmd := NewCmdAuthScopes(f, func(opts *ScopesOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"--format", "json"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Format != "json" {
		t.Errorf("expected format json, got %s", gotOpts.Format)
	}
}

func TestAuthScopesRun_UsesTenantAccessTokenFromCredentialProvider(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "", Brand: core.BrandFeishu,
	})
	tokenResolver := &authScopesTokenResolver{}
	f.Credential = credential.NewCredentialProvider(nil, nil, tokenResolver, nil)

	appInfoStub := &httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/application/v6/applications/test-app",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"app": map[string]interface{}{
					"creator_id": "ou_creator",
					"scopes": []map[string]interface{}{
						{
							"scope":       "im:message",
							"token_types": []string{"tenant"},
						},
						{
							"scope":       "im:message:send_as_user",
							"token_types": []string{"user"},
						},
					},
				},
			},
		},
	}
	reg.Register(appInfoStub)

	err := authScopesRun(&ScopesOptions{
		Factory: f,
		Ctx:     context.Background(),
		Format:  "json",
	})
	if err != nil {
		t.Fatalf("authScopesRun() error = %v", err)
	}

	if len(tokenResolver.requests) != 1 {
		t.Fatalf("resolved token requests = %v, want exactly one request", tokenResolver.requests)
	}
	if got := tokenResolver.requests[0].Type; got != credential.TokenTypeTAT {
		t.Fatalf("resolved token type = %q, want %q", got, credential.TokenTypeTAT)
	}
	if got := appInfoStub.CapturedHeaders.Get("Authorization"); got != "Bearer tenant-token" {
		t.Fatalf("Authorization header = %q, want %q", got, "Bearer tenant-token")
	}
}

type authScopesTokenResolver struct {
	requests []credential.TokenSpec
}

func (r *authScopesTokenResolver) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	r.requests = append(r.requests, req)
	switch req.Type {
	case credential.TokenTypeTAT:
		return &credential.TokenResult{Token: "tenant-token"}, nil
	case credential.TokenTypeUAT:
		return &credential.TokenResult{Token: "user-token"}, nil
	default:
		return &credential.TokenResult{Token: "unexpected-token"}, nil
	}
}
