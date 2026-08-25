// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestMeetingEndContract(t *testing.T) {
	if VCMeetingEnd.Risk != "high-risk-write" {
		t.Fatalf("Risk = %q, want high-risk-write", VCMeetingEnd.Risk)
	}
	if !VCMeetingEnd.ConfirmationBeforeNetwork {
		t.Fatal("ConfirmationBeforeNetwork = false, want true")
	}
	if !reflect.DeepEqual(VCMeetingEnd.Scopes, []string{}) {
		t.Fatalf("Scopes = %#v, want explicit empty slice", VCMeetingEnd.Scopes)
	}
	if len(VCMeetingEnd.ConditionalUserScopes) != 1 || VCMeetingEnd.ConditionalUserScopes[0] != "vc:meeting" {
		t.Fatalf("ConditionalUserScopes = %v, want [vc:meeting]", VCMeetingEnd.ConditionalUserScopes)
	}
	if len(VCMeetingEnd.ConditionalBotScopes) != 1 || VCMeetingEnd.ConditionalBotScopes[0] != "vc:meeting.bot.manage:write" {
		t.Fatalf("ConditionalBotScopes = %v, want [vc:meeting.bot.manage:write]", VCMeetingEnd.ConditionalBotScopes)
	}
	if got := VCMeetingEnd.DeclaredScopesForIdentity("user"); len(got) != 1 || got[0] != "vc:meeting" {
		t.Fatalf("DeclaredScopesForIdentity(user) = %v, want [vc:meeting]", got)
	}
	if got := VCMeetingEnd.DeclaredScopesForIdentity("bot"); len(got) != 1 || got[0] != "vc:meeting.bot.manage:write" {
		t.Fatalf("DeclaredScopesForIdentity(bot) = %v, want [vc:meeting.bot.manage:write]", got)
	}
	if !reflect.DeepEqual(VCMeetingEnd.AuthTypes, []string{"user", "bot"}) {
		t.Fatalf("AuthTypes = %v, want [user bot]", VCMeetingEnd.AuthTypes)
	}
}

type meetingManagementCountingTokenResolver struct {
	requests []credential.TokenSpec
	result   *credential.TokenResult
	err      error
}

func (r *meetingManagementCountingTokenResolver) ResolveToken(_ context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	r.requests = append(r.requests, req)
	if r.err != nil {
		return nil, r.err
	}
	if r.result != nil {
		resultCopy := *r.result
		return &resultCopy, nil
	}
	return &credential.TokenResult{Token: "counting-token"}, nil
}

type meetingManagementCountingAccountResolver struct {
	requests int
	account  *credential.Account
	err      error
}

func (r *meetingManagementCountingAccountResolver) ResolveAccount(_ context.Context) (*credential.Account, error) {
	r.requests++
	if r.err != nil {
		return nil, r.err
	}
	if r.account != nil {
		accountCopy := *r.account
		return &accountCopy, nil
	}
	return credential.AccountFromCliConfig(defaultConfig()), nil
}

func newMeetingManagementFactoryWithCounters(t *testing.T) (*cmdutil.Factory, *meetingManagementCountingAccountResolver, *meetingManagementCountingTokenResolver, *httpmock.Registry, *bytes.Buffer) {
	t.Helper()
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	accountResolver := &meetingManagementCountingAccountResolver{}
	tokenResolver := &meetingManagementCountingTokenResolver{}
	f.Credential = credential.NewCredentialProvider(nil, accountResolver, tokenResolver, nil)
	return f, accountResolver, tokenResolver, reg, stdout
}

func TestMeetingEndValidateMeetingID(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "positive", value: "7651377260537433044"},
		{name: "trimmed positive", value: " 7651377260537433044 "},
		{name: "nine digit positive remains valid", value: "123456789"},
		{name: "empty", value: "", wantErr: true},
		{name: "whitespace", value: "   ", wantErr: true},
		{name: "zero", value: "0", wantErr: true},
		{name: "negative", value: "-1", wantErr: true},
		{name: "non decimal", value: "0x10", wantErr: true},
		{name: "fraction", value: "1.5", wantErr: true},
		{name: "overflow", value: "9223372036854775808", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMeetingManagementID(tt.value)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("validateMeetingManagementID(%q) error = %v", tt.value, err)
				}
				return
			}

			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
			}
			if validationErr.Param != "--meeting-id" {
				t.Fatalf("Param = %q, want --meeting-id", validationErr.Param)
			}
		})
	}
}

func TestMeetingEndDryRunUsesEscapedPathWithoutAPICall(t *testing.T) {
	tests := []struct {
		identity string
		method   string
		url      string
		wantBody map[string]interface{}
	}{
		{identity: "user", method: http.MethodPatch, url: "/open-apis/vc/v1/meetings/7651377260537433044/end"},
		{identity: "bot", method: http.MethodPost, url: meetingBotEndPath, wantBody: map[string]interface{}{"meeting_id": "7651377260537433044"}},
	}

	for _, tt := range tests {
		t.Run(tt.identity, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
			calls := 0
			reg.Register(&httpmock.Stub{
				Method:   tt.method,
				URL:      tt.url,
				Optional: true,
				OnMatch: func(_ *http.Request) {
					calls++
				},
			})

			err := mountAndRun(t, VCMeetingEnd, []string{
				"+meeting-end", "--meeting-id", " 7651377260537433044 ",
				"--dry-run", "--as", tt.identity,
			}, f, stdout)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if calls != 0 {
				t.Fatalf("API calls = %d, want 0", calls)
			}

			var envelope struct {
				Data struct {
					API []struct {
						Method string                 `json:"method"`
						URL    string                 `json:"url"`
						Body   map[string]interface{} `json:"body"`
					} `json:"api"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
			}
			if len(envelope.Data.API) != 1 {
				t.Fatalf("api calls = %#v, want one call", envelope.Data.API)
			}
			call := envelope.Data.API[0]
			if call.Method != tt.method || call.URL != tt.url {
				t.Fatalf("dry-run call = %#v, want %s %s", call, tt.method, tt.url)
			}
			if !reflect.DeepEqual(call.Body, tt.wantBody) {
				t.Fatalf("dry-run body = %#v, want %#v", call.Body, tt.wantBody)
			}
		})
	}
}

func TestMeetingEndDryRunRequiresExplicitIdentity(t *testing.T) {
	for _, args := range [][]string{
		{"+meeting-end", "--meeting-id", "7651377260537433044", "--dry-run"},
		{"+meeting-end", "--meeting-id", "7651377260537433044", "--dry-run", "--as", "auto"},
	} {
		f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
		err := mountAndRun(t, VCMeetingEnd, args, f, stdout)
		var validationErr *errs.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("args %v error = %T %v, want *errs.ValidationError", args, err, err)
		}
		if validationErr.Param != "--as" {
			t.Fatalf("args %v Param = %q, want --as", args, validationErr.Param)
		}
	}
}

func TestMeetingEndRequiresConfirmationWithoutAPICall(t *testing.T) {
	f, accountResolver, resolver, reg, stdout := newMeetingManagementFactoryWithCounters(t)
	calls := 0
	reg.Register(&httpmock.Stub{
		Method:   "PATCH",
		URL:      "/open-apis/vc/v1/meetings/7651377260537433044/end",
		Optional: true,
		OnMatch: func(_ *http.Request) {
			calls++
		},
	})

	err := mountAndRun(t, VCMeetingEnd, []string{
		"+meeting-end", "--meeting-id", "7651377260537433044", "--as", "user",
	}, f, stdout)
	var confirmationErr *errs.ConfirmationRequiredError
	if !errors.As(err, &confirmationErr) {
		t.Fatalf("error = %T %v, want *errs.ConfirmationRequiredError", err, err)
	}
	if confirmationErr.Action != "vc +meeting-end" || confirmationErr.Risk != "high-risk-write" {
		t.Fatalf("confirmation = %#v", confirmationErr)
	}
	if accountResolver.requests != 0 {
		t.Fatalf("ResolveAccount calls = %d, want none before confirmation", accountResolver.requests)
	}
	if len(resolver.requests) != 0 {
		t.Fatalf("ResolveToken calls = %v, want none before confirmation", resolver.requests)
	}
	if calls != 0 {
		t.Fatalf("API calls = %d, want 0", calls)
	}
}

func TestMeetingEndDryRunDoesNotResolveScopesOrCallAPI(t *testing.T) {
	f, accountResolver, resolver, reg, stdout := newMeetingManagementFactoryWithCounters(t)
	calls := 0
	reg.Register(&httpmock.Stub{
		Method:   "PATCH",
		URL:      "/open-apis/vc/v1/meetings/7651377260537433044/end",
		Optional: true,
		OnMatch: func(_ *http.Request) {
			calls++
		},
	})

	err := mountAndRun(t, VCMeetingEnd, []string{
		"+meeting-end", "--meeting-id", "7651377260537433044",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if accountResolver.requests != 0 {
		t.Fatalf("ResolveAccount calls = %d, want none during dry-run", accountResolver.requests)
	}
	if len(resolver.requests) != 0 {
		t.Fatalf("ResolveToken calls = %v, want none during dry-run", resolver.requests)
	}
	if calls != 0 {
		t.Fatalf("API calls = %d, want 0", calls)
	}
}

func TestMeetingEndExecuteScopePreflightRunsBeforeAPI(t *testing.T) {
	f, accountResolver, _, reg, stdout := newMeetingManagementFactoryWithCounters(t)
	resolver := &meetingManagementCountingTokenResolver{
		result: &credential.TokenResult{Token: "uat-test", Scopes: "vc:record:readonly"},
	}
	f.Credential = credential.NewCredentialProvider(nil, accountResolver, resolver, nil)
	apiCalls := 0
	reg.Register(&httpmock.Stub{
		Method:   "PATCH",
		URL:      "/open-apis/vc/v1/meetings/7651377260537433044/end",
		Optional: true,
		OnMatch: func(_ *http.Request) {
			apiCalls++
		},
	})

	err := mountAndRun(t, VCMeetingEnd, []string{
		"+meeting-end", "--meeting-id", "7651377260537433044",
		"--yes", "--as", "user",
	}, f, stdout)
	var permissionErr *errs.PermissionError
	if !errors.As(err, &permissionErr) {
		t.Fatalf("error = %T %v, want *errs.PermissionError", err, err)
	}
	if permissionErr.Subtype != errs.SubtypeMissingScope {
		t.Fatalf("Subtype = %q, want %q", permissionErr.Subtype, errs.SubtypeMissingScope)
	}
	if permissionErr.Identity != string(core.AsUser) {
		t.Fatalf("Identity = %q, want %q", permissionErr.Identity, core.AsUser)
	}
	if len(permissionErr.MissingScopes) != 1 || permissionErr.MissingScopes[0] != "vc:meeting" {
		t.Fatalf("MissingScopes = %v, want [vc:meeting]", permissionErr.MissingScopes)
	}
	if accountResolver.requests != 1 {
		t.Fatalf("ResolveAccount calls = %d, want exactly one live config load", accountResolver.requests)
	}
	if len(resolver.requests) != 1 {
		t.Fatalf("ResolveToken calls = %v, want exactly one scope preflight call", resolver.requests)
	}
	if apiCalls != 0 {
		t.Fatalf("API calls = %d, want 0 because scope preflight must fail before HTTP", apiCalls)
	}
}

func TestMeetingEndExecuteUsesNilBodyAndTypedAPIError(t *testing.T) {
	tests := []struct {
		name        string
		response    map[string]interface{}
		wantErr     bool
		wantSubtype errs.Subtype
	}{
		{
			name: "success",
			response: map[string]interface{}{
				"code":        0,
				"msg":         "ok",
				"log_id":      "log-meeting-end-success",
				"server_meta": "preserved",
				"data":        map[string]interface{}{"ended": true, "server_detail": "preserved"},
			},
		},
		{
			name:        "permission denied",
			response:    map[string]interface{}{"code": 121005, "msg": "no permission", "log_id": "log-meeting-end"},
			wantErr:     true,
			wantSubtype: errs.SubtypePermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
			stub := &httpmock.Stub{
				Method: "PATCH",
				URL:    "/open-apis/vc/v1/meetings/7651377260537433044/end",
				Body:   tt.response,
				OnMatch: func(req *http.Request) {
					if req.Body == nil {
						return
					}
					body, readErr := io.ReadAll(req.Body)
					if readErr != nil {
						t.Errorf("read request body: %v", readErr)
					}
					if len(body) != 0 {
						t.Errorf("request body = %q, want empty", body)
					}
				},
			}
			reg.Register(stub)

			err := mountAndRun(t, VCMeetingEnd, []string{
				"+meeting-end", "--meeting-id", "7651377260537433044",
				"--yes", "--as", "user",
			}, f, stdout)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				var envelope struct {
					OK   bool `json:"ok"`
					Data struct {
						Code       int    `json:"code"`
						Msg        string `json:"msg"`
						LogID      string `json:"log_id"`
						ServerMeta string `json:"server_meta"`
						Data       struct {
							Ended        bool   `json:"ended"`
							ServerDetail string `json:"server_detail"`
						} `json:"data"`
					} `json:"data"`
				}
				if decodeErr := json.Unmarshal(stdout.Bytes(), &envelope); decodeErr != nil {
					t.Fatalf("decode output: %v\n%s", decodeErr, stdout.String())
				}
				if !envelope.OK || envelope.Data.Code != 0 || envelope.Data.Msg != "ok" ||
					envelope.Data.LogID != "log-meeting-end-success" || envelope.Data.ServerMeta != "preserved" ||
					!envelope.Data.Data.Ended || envelope.Data.Data.ServerDetail != "preserved" {
					t.Fatalf("envelope = %#v, want complete server envelope and data", envelope)
				}
				return
			}

			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Subtype != tt.wantSubtype || problem.LogID != "log-meeting-end" {
				t.Fatalf("error = %T %v, problem = %#v", err, err, problem)
			}
		})
	}
}

func TestMeetingEndBotExecuteUsesPostBodyAndDataOutput(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	stub := &httpmock.Stub{
		Method: http.MethodPost,
		URL:    meetingBotEndPath,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"server_detail": "preserved",
			},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, VCMeetingEnd, []string{
		"+meeting-end", "--meeting-id", " 7628568141510692381 ",
		"--yes", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if !reflect.DeepEqual(body, map[string]interface{}{"meeting_id": "7628568141510692381"}) {
		t.Fatalf("request body = %#v, want meeting_id only", body)
	}

	var envelope struct {
		OK       bool                   `json:"ok"`
		Identity string                 `json:"identity"`
		Data     map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	wantData := map[string]interface{}{
		"meeting_id":    "7628568141510692381",
		"server_detail": "preserved",
	}
	if !envelope.OK || envelope.Identity != "bot" || !reflect.DeepEqual(envelope.Data, wantData) {
		t.Fatalf("envelope = %#v, want bot data output %#v", envelope, wantData)
	}
}

func TestBuildMeetingEndPathTrimsAndEscapesMeetingID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trim", in: " 7651377260537433044 ", want: "/open-apis/vc/v1/meetings/7651377260537433044/end"},
		{name: "escape segment", in: "1/2", want: "/open-apis/vc/v1/meetings/1%2F2/end"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildMeetingEndPath(tt.in); got != tt.want {
				t.Fatalf("buildMeetingEndPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
