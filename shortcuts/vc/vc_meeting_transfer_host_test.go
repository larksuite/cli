// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

const meetingTransferHostTestPath = "/open-apis/vc/v1/meetings/7651377260537433044/set_host"

func TestMeetingTransferHostContract(t *testing.T) {
	if VCMeetingTransferHost.Risk != "high-risk-write" {
		t.Fatalf("Risk = %q, want high-risk-write", VCMeetingTransferHost.Risk)
	}
	if !reflect.DeepEqual(VCMeetingTransferHost.Scopes, []string{}) {
		t.Fatalf("Scopes = %#v, want explicit empty slice", VCMeetingTransferHost.Scopes)
	}
	if got := VCMeetingTransferHost.DeclaredScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"vc:meeting"}) {
		t.Fatalf("DeclaredScopesForIdentity(user) = %v, want [vc:meeting]", got)
	}
	if !reflect.DeepEqual(VCMeetingTransferHost.AuthTypes, []string{"user"}) {
		t.Fatalf("AuthTypes = %v, want [user]", VCMeetingTransferHost.AuthTypes)
	}
	var sawParticipant, sawUserIDType bool
	for _, flag := range VCMeetingTransferHost.Flags {
		switch flag.Name {
		case "participant":
			sawParticipant = true
			if flag.Type != "" || !flag.Required {
				t.Fatalf("participant flag = %#v, want required single string", flag)
			}
		case "user-id-type":
			sawUserIDType = true
			if flag.Default != "open_id" || !reflect.DeepEqual(flag.Enum, []string{"open_id", "union_id", "user_id"}) {
				t.Fatalf("user-id-type flag = %#v, want default open_id and expected enum", flag)
			}
		}
	}
	if !sawParticipant {
		t.Fatal("participant flag is not declared")
	}
	if !sawUserIDType {
		t.Fatal("user-id-type flag is not declared")
	}
}

func TestMeetingTransferHostDryRunWithoutAPICall(t *testing.T) {
	f, accountResolver, resolver, reg, stdout := newMeetingManagementFactoryWithCounters(t)
	calls := 0
	reg.Register(&httpmock.Stub{
		Method:   "PATCH",
		URL:      meetingTransferHostTestPath,
		Optional: true,
		OnMatch: func(_ *http.Request) {
			calls++
		},
	})

	err := mountAndRun(t, VCMeetingTransferHost, []string{
		"+meeting-transfer-host",
		"--meeting-id", "7651377260537433044",
		"--participant", "ou_123=1",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("API calls = %d, want 0", calls)
	}
	if accountResolver.requests != 1 {
		t.Fatalf("ResolveAccount calls = %d, want one config load during dry-run", accountResolver.requests)
	}
	if len(resolver.requests) != 0 {
		t.Fatalf("ResolveToken calls = %v, want none during dry-run", resolver.requests)
	}

	var envelope struct {
		Data struct {
			API []struct {
				Method string                  `json:"method"`
				URL    string                  `json:"url"`
				Params map[string]interface{}  `json:"params"`
				Body   meetingTransferHostBody `json:"body"`
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
	if call.Method != "PATCH" || call.URL != meetingTransferHostTestPath {
		t.Fatalf("dry-run call = %#v", call)
	}
	if call.Params["user_id_type"] != "open_id" {
		t.Fatalf("dry-run params = %#v, want user_id_type=open_id", call.Params)
	}
	if want := (meetingTransferHostUser{ID: "ou_123", UserType: 1}); call.Body.HostUser != want {
		t.Fatalf("host_user = %#v, want %#v", call.Body.HostUser, want)
	}
}

func TestMeetingTransferHostValidationStopsBeforeAnyAPICall(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantParam string
	}{
		{
			name: "invalid meeting ID",
			args: []string{
				"+meeting-transfer-host",
				"--meeting-id", "meeting_123",
				"--participant", "ou_123=1",
				"--yes", "--as", "user",
			},
			wantParam: "--meeting-id",
		},
		{
			name: "malformed participant tuple",
			args: []string{
				"+meeting-transfer-host",
				"--meeting-id", "7651377260537433044",
				"--participant", "ou_123",
				"--yes", "--as", "user",
			},
			wantParam: "--participant",
		},
		{
			name: "participant user_type must be in documented range",
			args: []string{
				"+meeting-transfer-host",
				"--meeting-id", "7651377260537433044",
				"--participant", "ou_123=8",
				"--yes", "--as", "user",
			},
			wantParam: "--participant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, resolver, reg, stdout := newMeetingManagementFactoryWithCounters(t)
			calls := 0
			reg.Register(&httpmock.Stub{
				Method:   "PATCH",
				URL:      "/set_host",
				Optional: true,
				OnMatch: func(_ *http.Request) {
					calls++
				},
			})

			err := mountAndRun(t, VCMeetingTransferHost, tt.args, f, stdout)
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
			}
			if validationErr.Param != tt.wantParam {
				t.Fatalf("Param = %q, want %s", validationErr.Param, tt.wantParam)
			}
			if len(resolver.requests) != 0 {
				t.Fatalf("ResolveToken calls = %v, want none", resolver.requests)
			}
			if calls != 0 {
				t.Fatalf("API calls = %d, want 0", calls)
			}
		})
	}
}

func TestMeetingTransferHostRequiresConfirmationWithoutAPICall(t *testing.T) {
	f, _, resolver, reg, stdout := newMeetingManagementFactoryWithCounters(t)
	calls := 0
	reg.Register(&httpmock.Stub{
		Method:   "PATCH",
		URL:      meetingTransferHostTestPath,
		Optional: true,
		OnMatch: func(_ *http.Request) {
			calls++
		},
	})

	err := mountAndRun(t, VCMeetingTransferHost, []string{
		"+meeting-transfer-host",
		"--meeting-id", "7651377260537433044",
		"--participant", "ou_123=1",
		"--as", "user",
	}, f, stdout)
	var confirmationErr *errs.ConfirmationRequiredError
	if !errors.As(err, &confirmationErr) {
		t.Fatalf("error = %T %v, want *errs.ConfirmationRequiredError", err, err)
	}
	if confirmationErr.Action != "vc +meeting-transfer-host" || confirmationErr.Risk != "high-risk-write" {
		t.Fatalf("confirmation = %#v", confirmationErr)
	}
	if len(resolver.requests) != 0 {
		t.Fatalf("ResolveToken calls = %v, want none before confirmation", resolver.requests)
	}
	if calls != 0 {
		t.Fatalf("API calls = %d, want 0", calls)
	}
}

func TestMeetingTransferHostExecuteScopePreflightRunsBeforeAPI(t *testing.T) {
	f, accountResolver, _, reg, stdout := newMeetingManagementFactoryWithCounters(t)
	resolver := &meetingManagementCountingTokenResolver{
		result: &credential.TokenResult{Token: "uat-test", Scopes: "vc:record:readonly"},
	}
	f.Credential = credential.NewCredentialProvider(nil, accountResolver, resolver, nil)
	apiCalls := 0
	reg.Register(&httpmock.Stub{
		Method:   "PATCH",
		URL:      meetingTransferHostTestPath,
		Optional: true,
		OnMatch: func(_ *http.Request) {
			apiCalls++
		},
	})

	err := mountAndRun(t, VCMeetingTransferHost, []string{
		"+meeting-transfer-host",
		"--meeting-id", "7651377260537433044",
		"--participant", "ou_123=1",
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
	if apiCalls != 0 {
		t.Fatalf("API calls = %d, want 0 because scope preflight must fail before HTTP", apiCalls)
	}
}

func TestMeetingTransferHostExecutePreservesRequestAndServerEnvelope(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    meetingTransferHostTestPath,
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("user_id_type"); got != "union_id" {
				t.Fatalf("user_id_type = %q, want union_id", got)
			}
		},
		Body: map[string]interface{}{
			"code":   0,
			"msg":    "ok",
			"log_id": "log-meeting-transfer-host-success",
			"data": map[string]interface{}{
				"host_user": map[string]interface{}{"id": "on_123", "user_type": 1},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, VCMeetingTransferHost, []string{
		"+meeting-transfer-host",
		"--meeting-id", "7651377260537433044",
		"--user-id-type", "union_id",
		"--participant", "on_123=1",
		"--yes", "--format", "json", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var request meetingTransferHostBody
	if err := json.Unmarshal(stub.CapturedBody, &request); err != nil {
		t.Fatalf("decode request: %v\n%s", err, stub.CapturedBody)
	}
	if want := (meetingTransferHostUser{ID: "on_123", UserType: 1}); request.HostUser != want {
		t.Fatalf("request host_user = %#v, want %#v", request.HostUser, want)
	}

	var outputEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Code  int    `json:"code"`
			LogID string `json:"log_id"`
			Data  struct {
				HostUser meetingTransferHostUser `json:"host_user"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &outputEnvelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if !outputEnvelope.OK || outputEnvelope.Data.Code != 0 || outputEnvelope.Data.LogID != "log-meeting-transfer-host-success" ||
		outputEnvelope.Data.Data.HostUser != request.HostUser {
		t.Fatalf("envelope = %#v, want complete server envelope and host_user", outputEnvelope)
	}
}

func TestMeetingTransferHostExecutionIdentityAndErrors(t *testing.T) {
	tests := []struct {
		name     string
		agent    bool
		as       string
		code     int
		logIDAt  string
		category errs.Category
	}{
		{name: "agent uses user identity", agent: true, as: "user"},
		{name: "bot refused before API", as: "bot", category: errs.CategoryValidation},
		{name: "target cannot be host", as: "user", code: 122023, logIDAt: "nested", category: errs.CategoryAPI},
		{name: "RTC quota insufficient", as: "user", code: 122024, logIDAt: "body", category: errs.CategoryAPI},
		{name: "operator not in meeting", as: "user", code: 122003, logIDAt: "nested", category: errs.CategoryAPI},
		{name: "target not in meeting", as: "user", code: 122004, logIDAt: "nested", category: errs.CategoryAPI},
		{name: "permission denied", as: "user", code: 121005, logIDAt: "nested", category: errs.CategoryAuthorization},
		{name: "unknown server error with header log ID", as: "user", code: 987654, logIDAt: "header", category: errs.CategoryAPI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			cfg := defaultConfig()
			if tt.agent {
				cfg.CredentialKind = core.CredentialAAT
				cfg.PrincipalOpenID = "ou_testagent"
				cfg.UserOpenId = cfg.PrincipalOpenID
			}
			f, stdout, _, reg := cmdutil.TestFactory(t, cfg)
			var agentResolver *meetingTransferHostAgentTokenResolver
			wantToken := "test-token"
			if tt.agent {
				agentResolver = &meetingTransferHostAgentTokenResolver{}
				accountResolver := &meetingManagementCountingAccountResolver{account: credential.AccountFromCliConfig(cfg)}
				f.Credential = credential.NewCredentialProvider(nil, accountResolver, agentResolver, f.HttpClient)
				wantToken = "test-aat"
			}
			const logID = "host-transfer-error-log"
			body := map[string]interface{}{
				"code": tt.code,
				"msg":  "host transfer response",
				"data": map[string]interface{}{
					"host_user": map[string]interface{}{"id": "ou_target", "user_type": 1},
				},
			}
			headers := http.Header{"Content-Type": []string{"application/json"}}
			switch tt.logIDAt {
			case "nested":
				body["error"] = map[string]interface{}{"log_id": logID}
			case "body":
				body["log_id"] = logID
			case "header":
				headers.Set("X-Tt-Logid", logID)
			}
			stub := &httpmock.Stub{
				Method: "PATCH", URL: meetingTransferHostTestPath, Optional: tt.as == "bot",
				Body: body, Headers: headers,
				OnMatch: func(req *http.Request) {
					if got := req.Header.Get("Authorization"); got != "Bearer "+wantToken {
						t.Fatalf("Authorization = %q, want requested identity's token", got)
					}
					if got := req.URL.Query().Get("user_id_type"); got != "open_id" {
						t.Fatalf("user_id_type = %q, want open_id", got)
					}
				},
			}
			reg.Register(stub)
			err := mountAndRun(t, VCMeetingTransferHost, []string{
				"+meeting-transfer-host", "--meeting-id", "7651377260537433044",
				"--participant", "ou_target=1", "--as", tt.as, "--yes", "--format", "json",
			}, f, stdout)
			if tt.as == "bot" {
				if len(stub.CapturedBodies) != 0 {
					t.Fatal("bot invocation must not call the API")
				}
			} else {
				if len(stub.CapturedBodies) != 1 {
					t.Fatalf("API calls = %d, want 1", len(stub.CapturedBodies))
				}
				var request meetingTransferHostBody
				if err := json.Unmarshal(stub.CapturedBody, &request); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if request.HostUser != (meetingTransferHostUser{ID: "ou_target", UserType: 1}) {
					t.Fatalf("host_user = %#v, want requested successor", request.HostUser)
				}
			}
			if tt.category != "" {
				problem, ok := errs.ProblemOf(err)
				if !ok || problem.Category != tt.category || problem.Code != tt.code {
					t.Fatalf("error = %T %v, want category=%s code=%d", err, err, tt.category, tt.code)
				}
				if tt.code != 0 && problem.LogID != logID {
					t.Fatalf("LogID = %q, want %q", problem.LogID, logID)
				}
				if stdout.Len() != 0 {
					t.Fatalf("failed response with host_user must not print success: %s", stdout.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("AAT execution failed: %v", err)
			}
			if len(agentResolver.requests) == 0 {
				t.Fatal("AAT execution must resolve Agent Employee access")
			}
			for _, request := range agentResolver.requests {
				if request.Identity != core.AsUser || request.AppID != cfg.AppID {
					t.Fatalf("access request = %#v, want user identity for configured app", request)
				}
			}
			var result struct {
				OK       bool   `json:"ok"`
				Identity string `json:"identity"`
				Data     struct {
					Data struct {
						HostUser meetingTransferHostUser `json:"host_user"`
					} `json:"data"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("decode AAT result: %v", err)
			}
			if !result.OK || result.Identity != "user" || result.Data.Data.HostUser != (meetingTransferHostUser{ID: "ou_target", UserType: 1}) {
				t.Fatalf("AAT result = %#v, want successful user result with requested host", result)
			}
		})
	}
}

type meetingTransferHostAgentTokenResolver struct {
	requests []credential.AccessSpec
}

func (*meetingTransferHostAgentTokenResolver) ResolveToken(context.Context, credential.TokenSpec) (*credential.TokenResult, error) {
	return nil, errs.NewInternalError(errs.SubtypeSDKError, "Agent Employee must use access resolution")
}

func (r *meetingTransferHostAgentTokenResolver) ResolveAccess(_ context.Context, account *credential.Account, spec credential.AccessSpec) (*credential.ResolvedAccess, bool, error) {
	r.requests = append(r.requests, spec)
	return &credential.ResolvedAccess{
		Value: "test-aat", Kind: core.CredentialAAT, OpenID: account.PrincipalOpenID,
		Scopes: "vc:meeting", ExpiresAt: time.Now().Add(time.Hour),
	}, true, nil
}

func TestBuildMeetingTransferHostBody(t *testing.T) {
	tests := []struct {
		name        string
		participant string
		wantType    int
	}{
		{name: "minimum user type", participant: "ou_target=1", wantType: 1},
		{name: "maximum user type", participant: "ou_target=7", wantType: 7},
		{name: "empty tuple"},
		{name: "missing separator", participant: "ou_target"},
		{name: "extra separator", participant: "ou_target=1=2"},
		{name: "empty ID", participant: "=1"},
		{name: "ID leading whitespace", participant: " ou_target=1"},
		{name: "ID trailing whitespace", participant: "ou_target =1"},
		{name: "empty user type", participant: "ou_target="},
		{name: "noninteger user type", participant: "ou_target=one"},
		{name: "user type below minimum", participant: "ou_target=0"},
		{name: "user type above maximum", participant: "ou_target=8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "+meeting-transfer-host"}
			cmd.Flags().String("participant", tt.participant, "")
			runtime := common.TestNewRuntimeContextWithIdentity(cmd, defaultConfig(), core.AsUser)
			body, err := buildMeetingTransferHostBody(runtime)
			if tt.wantType == 0 {
				var validationErr *errs.ValidationError
				if !errors.As(err, &validationErr) || validationErr.Param != "--participant" {
					t.Fatalf("error = %T %v, want participant validation error", err, err)
				}
				return
			}
			if err != nil || body.HostUser != (meetingTransferHostUser{ID: "ou_target", UserType: tt.wantType}) {
				t.Fatalf("body = %#v, error = %v, want requested participant", body, err)
			}
		})
	}
}
