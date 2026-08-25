// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestMeetingParticipantKickoutContract(t *testing.T) {
	if VCMeetingParticipantKickout.Risk != "high-risk-write" {
		t.Fatalf("Risk = %q, want high-risk-write", VCMeetingParticipantKickout.Risk)
	}
	if !VCMeetingParticipantKickout.ConfirmationBeforeNetwork {
		t.Fatal("ConfirmationBeforeNetwork = false, want true")
	}
	if !reflect.DeepEqual(VCMeetingParticipantKickout.Scopes, []string{}) {
		t.Fatalf("Scopes = %#v, want explicit empty slice", VCMeetingParticipantKickout.Scopes)
	}
	if !reflect.DeepEqual(VCMeetingParticipantKickout.ConditionalUserScopes, []string{"vc:meeting"}) {
		t.Fatalf("ConditionalUserScopes = %v, want [vc:meeting]", VCMeetingParticipantKickout.ConditionalUserScopes)
	}
	if got := VCMeetingParticipantKickout.DeclaredScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"vc:meeting"}) {
		t.Fatalf("DeclaredScopesForIdentity(user) = %v, want [vc:meeting]", got)
	}
	if !reflect.DeepEqual(VCMeetingParticipantKickout.AuthTypes, []string{"user"}) {
		t.Fatalf("AuthTypes = %v, want [user]", VCMeetingParticipantKickout.AuthTypes)
	}
	for _, flag := range VCMeetingParticipantKickout.Flags {
		if flag.Name == "participant" {
			if flag.Type != "string_array" || !flag.Required {
				t.Fatalf("participant flag = %#v, want required string_array", flag)
			}
			return
		}
	}
	t.Fatal("participant flag is not declared")
}

func TestParseMeetingParticipantKickoutUsersPreservesOrderDuplicatesAndStringIDs(t *testing.T) {
	input := []string{"000123=1", "000123=2", "000123=1"}
	want := []meetingParticipantKickoutUser{
		{ID: "000123", UserType: 1},
		{ID: "000123", UserType: 2},
		{ID: "000123", UserType: 1},
	}

	got, err := parseMeetingParticipantKickoutUsers(input)
	if err != nil {
		t.Fatalf("parseMeetingParticipantKickoutUsers() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("users = %#v, want %#v", got, want)
	}
}

func TestParseMeetingParticipantKickoutUsersValidation(t *testing.T) {
	validTen := []string{"1=1", "2=2", "3=3", "4=4", "5=5", "6=6", "7=7", "8=1", "9=2", "10=3"}
	tests := []struct {
		name   string
		values []string
	}{
		{name: "missing", values: nil},
		{name: "too many", values: append(append([]string{}, validTen...), "11=4")},
		{name: "missing equals", values: []string{"123"}},
		{name: "multiple equals", values: []string{"123=1=2"}},
		{name: "empty id", values: []string{" =1"}},
		{name: "id has leading whitespace", values: []string{" 000123=1"}},
		{name: "id has trailing whitespace", values: []string{"000123 =1"}},
		{name: "id zero", values: []string{"0=1"}},
		{name: "id negative", values: []string{"-1=1"}},
		{name: "id non decimal", values: []string{"12a=1"}},
		{name: "id overflow", values: []string{"9223372036854775808=1"}},
		{name: "empty user type", values: []string{"123="}},
		{name: "non integer user type", values: []string{"123=user"}},
		{name: "user type below range", values: []string{"123=0"}},
		{name: "user type above range", values: []string{"123=8"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseMeetingParticipantKickoutUsers(tt.values)
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
			}
			if validationErr.Param != "--participant" {
				t.Fatalf("Param = %q, want --participant", validationErr.Param)
			}
		})
	}

	got, err := parseMeetingParticipantKickoutUsers(validTen)
	if err != nil || len(got) != maxMeetingKickoutParticipants {
		t.Fatalf("ten participants = %#v, %v", got, err)
	}
}

func TestMeetingParticipantKickoutDryRunPreservesTuplesWithoutAPICall(t *testing.T) {
	f, accountResolver, resolver, reg, stdout := newMeetingManagementFactoryWithCounters(t)
	calls := 0
	reg.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/vc/v1/meetings/7651377260537433044/kickout",
		Optional: true,
		OnMatch: func(_ *http.Request) {
			calls++
		},
	})

	err := mountAndRun(t, VCMeetingParticipantKickout, []string{
		"+meeting-participant-kickout",
		"--meeting-id", "7651377260537433044",
		"--participant", "000123=1",
		"--participant", "000123=2",
		"--participant", "000123=1",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("API calls = %d, want 0", calls)
	}
	if accountResolver.requests != 0 {
		t.Fatalf("ResolveAccount calls = %d, want none during dry-run", accountResolver.requests)
	}
	if len(resolver.requests) != 0 {
		t.Fatalf("ResolveToken calls = %v, want none during dry-run", resolver.requests)
	}

	var envelope struct {
		Data struct {
			API []struct {
				Method string                        `json:"method"`
				URL    string                        `json:"url"`
				Body   meetingParticipantKickoutBody `json:"body"`
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
	if call.Method != "POST" || call.URL != "/open-apis/vc/v1/meetings/7651377260537433044/kickout" {
		t.Fatalf("dry-run call = %#v", call)
	}
	wantUsers := []meetingParticipantKickoutUser{
		{ID: "000123", UserType: 1},
		{ID: "000123", UserType: 2},
		{ID: "000123", UserType: 1},
	}
	if !reflect.DeepEqual(call.Body.KickoutUsers, wantUsers) {
		t.Fatalf("kickout_users = %#v, want %#v", call.Body.KickoutUsers, wantUsers)
	}
}

func TestMeetingParticipantKickoutValidationStopsBeforeAnyAPICall(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		wantRequiredError bool
	}{
		{
			name: "malformed participant tuple",
			args: []string{
				"+meeting-participant-kickout",
				"--meeting-id", "7651377260537433044",
				"--participant", " 000123=1",
				"--yes", "--as", "user",
			},
		},
		{
			name: "participant id must be positive base-10 int64",
			args: []string{
				"+meeting-participant-kickout",
				"--meeting-id", "7651377260537433044",
				"--participant", "0=1",
				"--yes", "--as", "user",
			},
		},
		{
			name:              "missing participant",
			wantRequiredError: true,
			args: []string{
				"+meeting-participant-kickout",
				"--meeting-id", "7651377260537433044",
				"--yes", "--as", "user",
			},
		},
		{
			name: "too many participants",
			args: []string{
				"+meeting-participant-kickout",
				"--meeting-id", "7651377260537433044",
				"--participant", "1=1",
				"--participant", "2=1",
				"--participant", "3=1",
				"--participant", "4=1",
				"--participant", "5=1",
				"--participant", "6=1",
				"--participant", "7=1",
				"--participant", "8=1",
				"--participant", "9=1",
				"--participant", "10=1",
				"--participant", "11=1",
				"--yes", "--as", "user",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, accountResolver, resolver, reg, stdout := newMeetingManagementFactoryWithCounters(t)
			calls := 0
			reg.Register(&httpmock.Stub{
				Method:   "POST",
				URL:      "/open-apis/vc/v1/meetings/7651377260537433044/kickout",
				Optional: true,
				OnMatch: func(_ *http.Request) {
					calls++
				},
			})

			err := mountAndRun(t, VCMeetingParticipantKickout, tt.args, f, stdout)
			if tt.wantRequiredError {
				if err == nil || err.Error() != `required flag(s) "participant" not set` {
					t.Fatalf("error = %T %v, want Cobra required-flag error", err, err)
				}
			} else {
				var validationErr *errs.ValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
				}
				if validationErr.Param != "--participant" {
					t.Fatalf("Param = %q, want --participant", validationErr.Param)
				}
			}
			if accountResolver.requests != 0 {
				t.Fatalf("ResolveAccount calls = %d, want 0", accountResolver.requests)
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

func TestMeetingParticipantKickoutRequiresConfirmationWithoutAPICall(t *testing.T) {
	f, accountResolver, resolver, reg, stdout := newMeetingManagementFactoryWithCounters(t)
	calls := 0
	reg.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/vc/v1/meetings/7651377260537433044/kickout",
		Optional: true,
		OnMatch: func(_ *http.Request) {
			calls++
		},
	})

	err := mountAndRun(t, VCMeetingParticipantKickout, []string{
		"+meeting-participant-kickout",
		"--meeting-id", "7651377260537433044",
		"--participant", "000123=1",
		"--as", "user",
	}, f, stdout)
	var confirmationErr *errs.ConfirmationRequiredError
	if !errors.As(err, &confirmationErr) {
		t.Fatalf("error = %T %v, want *errs.ConfirmationRequiredError", err, err)
	}
	if confirmationErr.Action != "vc +meeting-participant-kickout" || confirmationErr.Risk != "high-risk-write" {
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

func TestMeetingParticipantKickoutExecuteScopePreflightRunsBeforeAPI(t *testing.T) {
	f, accountResolver, _, reg, stdout := newMeetingManagementFactoryWithCounters(t)
	resolver := &meetingManagementCountingTokenResolver{
		result: &credential.TokenResult{Token: "uat-test", Scopes: "vc:record:readonly"},
	}
	f.Credential = credential.NewCredentialProvider(nil, accountResolver, resolver, nil)
	apiCalls := 0
	reg.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/vc/v1/meetings/7651377260537433044/kickout",
		Optional: true,
		OnMatch: func(_ *http.Request) {
			apiCalls++
		},
	})

	err := mountAndRun(t, VCMeetingParticipantKickout, []string{
		"+meeting-participant-kickout",
		"--meeting-id", "7651377260537433044",
		"--participant", "000123=1",
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

func TestMeetingParticipantKickoutExecutePreservesRequestAndServerResults(t *testing.T) {
	for _, format := range []string{"json", "pretty"} {
		t.Run(format, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
			stub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/vc/v1/meetings/7651377260537433044/kickout",
				Body: map[string]interface{}{
					"code":         0,
					"msg":          "ok",
					"log_id":       "log-meeting-kickout-success",
					"server_trace": "preserved",
					"data": map[string]interface{}{
						"server_page": "preserved",
						"kickout_results": []interface{}{
							map[string]interface{}{"id": "000456", "user_type": 2, "result": map[string]interface{}{"future_code": 9}, "server_detail": "second-first"},
							map[string]interface{}{"id": "000123", "user_type": 1, "result": "future-result", "server_detail": "first-second"},
						},
					},
				},
			}
			reg.Register(stub)

			err := mountAndRun(t, VCMeetingParticipantKickout, []string{
				"+meeting-participant-kickout",
				"--meeting-id", "7651377260537433044",
				"--participant", "000123=1",
				"--participant", "000456=2",
				"--participant", "000123=1",
				"--yes", "--format", format, "--as", "user",
			}, f, stdout)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var request meetingParticipantKickoutBody
			if err := json.Unmarshal(stub.CapturedBody, &request); err != nil {
				t.Fatalf("decode request: %v\n%s", err, stub.CapturedBody)
			}
			wantUsers := []meetingParticipantKickoutUser{
				{ID: "000123", UserType: 1},
				{ID: "000456", UserType: 2},
				{ID: "000123", UserType: 1},
			}
			if !reflect.DeepEqual(request.KickoutUsers, wantUsers) {
				t.Fatalf("request kickout_users = %#v, want %#v", request.KickoutUsers, wantUsers)
			}

			var outputEnvelope struct {
				OK   bool `json:"ok"`
				Data struct {
					Code        int    `json:"code"`
					Msg         string `json:"msg"`
					LogID       string `json:"log_id"`
					ServerTrace string `json:"server_trace"`
					Data        struct {
						ServerPage     string `json:"server_page"`
						KickoutResults []struct {
							ID           string      `json:"id"`
							UserType     int         `json:"user_type"`
							Result       interface{} `json:"result"`
							ServerDetail string      `json:"server_detail"`
						} `json:"kickout_results"`
					} `json:"data"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &outputEnvelope); err != nil {
				t.Fatalf("decode %s output: %v\n%s", format, err, stdout.String())
			}
			if !outputEnvelope.OK || outputEnvelope.Data.Code != 0 || outputEnvelope.Data.Msg != "ok" ||
				outputEnvelope.Data.LogID != "log-meeting-kickout-success" || outputEnvelope.Data.ServerTrace != "preserved" ||
				outputEnvelope.Data.Data.ServerPage != "preserved" || len(outputEnvelope.Data.Data.KickoutResults) != 2 {
				t.Fatalf("envelope = %#v, want complete server envelope and data", outputEnvelope)
			}
			results := outputEnvelope.Data.Data.KickoutResults
			if results[0].ID != "000456" || results[0].UserType != 2 || results[0].ServerDetail != "second-first" ||
				!reflect.DeepEqual(results[0].Result, map[string]interface{}{"future_code": float64(9)}) ||
				results[1].ID != "000123" || results[1].UserType != 1 || results[1].Result != "future-result" || results[1].ServerDetail != "first-second" {
				t.Fatalf("kickout_results = %#v, want server order and opaque result values preserved after request deduplication", results)
			}
		})
	}
}

func TestValidateMeetingParticipantKickoutResponseTreatsResultAsOpaque(t *testing.T) {
	requested := []meetingParticipantKickoutUser{{ID: "123", UserType: 1}}
	for _, result := range []interface{}{7, "future-result", map[string]interface{}{"code": 9}, nil} {
		data := map[string]interface{}{
			"kickout_results": []interface{}{
				map[string]interface{}{"id": "123", "user_type": 1, "result": result},
			},
		}
		if err := validateMeetingParticipantKickoutResponse(data, requested); err != nil {
			t.Fatalf("result %#v was interpreted instead of treated as opaque: %v", result, err)
		}
	}
}

func TestValidateMeetingParticipantKickoutResponseUsesNormalizedTupleBijection(t *testing.T) {
	duplicateRequest := []meetingParticipantKickoutUser{
		{ID: "123", UserType: 1},
		{ID: "123", UserType: 1},
	}
	if err := validateMeetingParticipantKickoutResponse(
		map[string]interface{}{"kickout_results": []interface{}{
			map[string]interface{}{"id": "123", "user_type": 1, "result": "future-result"},
		}},
		duplicateRequest,
	); err != nil {
		t.Fatalf("duplicate request normalized to one server result was rejected: %v", err)
	}

	tests := []struct {
		name      string
		requested []meetingParticipantKickoutUser
		results   []interface{}
	}{
		{
			name: "distinct requests cannot be satisfied by a duplicate response",
			requested: []meetingParticipantKickoutUser{
				{ID: "123", UserType: 1},
				{ID: "456", UserType: 2},
			},
			results: []interface{}{
				map[string]interface{}{"id": "123", "user_type": 1, "result": "future-result"},
				map[string]interface{}{"id": "123", "user_type": 1, "result": 7},
			},
		},
		{
			name: "duplicate normalized request cannot be satisfied by an extra response",
			requested: []meetingParticipantKickoutUser{
				{ID: "123", UserType: 1},
				{ID: "123", UserType: 1},
			},
			results: []interface{}{
				map[string]interface{}{"id": "123", "user_type": 1, "result": map[string]interface{}{"code": 9}},
				map[string]interface{}{"id": "456", "user_type": 2, "result": nil},
			},
		},
		{
			name: "conflicting user types cannot be collapsed in a success response",
			requested: []meetingParticipantKickoutUser{
				{ID: "123", UserType: 1},
				{ID: "123", UserType: 2},
			},
			results: []interface{}{
				map[string]interface{}{"id": "123", "user_type": 1, "result": "future-result"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMeetingParticipantKickoutResponse(
				map[string]interface{}{"kickout_results": tt.results},
				tt.requested,
			)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("error = %T %v, problem = %#v, want internal/invalid_response", err, err, problem)
			}
		})
	}
}

func TestMeetingParticipantKickoutExecuteReturnsTypedAPIError(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/meetings/7651377260537433044/kickout",
		Body:   map[string]interface{}{"code": 121005, "msg": "no permission", "log_id": "log-meeting-kickout"},
	})

	err := mountAndRun(t, VCMeetingParticipantKickout, []string{
		"+meeting-participant-kickout",
		"--meeting-id", "7651377260537433044",
		"--participant", "000123=1",
		"--yes", "--as", "user",
	}, f, stdout)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypePermissionDenied || problem.LogID != "log-meeting-kickout" {
		t.Fatalf("error = %T %v, problem = %#v", err, err, problem)
	}
}

func TestMeetingParticipantKickoutExecuteRejectsMalformedSuccessData(t *testing.T) {
	tests := []struct {
		name string
		data interface{}
	}{
		{name: "missing data", data: nil},
		{name: "missing results", data: map[string]interface{}{}},
		{name: "empty results", data: map[string]interface{}{"kickout_results": []interface{}{}}},
		{name: "missing tuple field", data: map[string]interface{}{"kickout_results": []interface{}{map[string]interface{}{"id": "000123", "result": 1}}}},
		{name: "missing result field", data: map[string]interface{}{"kickout_results": []interface{}{map[string]interface{}{"id": "000123", "user_type": 1}}}},
		{name: "unrequested tuple", data: map[string]interface{}{"kickout_results": []interface{}{map[string]interface{}{"id": "999", "user_type": 1, "result": 1}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
			body := map[string]interface{}{"code": 0, "msg": "ok"}
			if tt.data != nil {
				body["data"] = tt.data
			}
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/vc/v1/meetings/7651377260537433044/kickout",
				Body:   body,
			})

			err := mountAndRun(t, VCMeetingParticipantKickout, []string{
				"+meeting-participant-kickout",
				"--meeting-id", "7651377260537433044",
				"--participant", "000123=1",
				"--yes", "--as", "user",
			}, f, stdout)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("error = %T %v, problem = %#v, want internal/invalid_response", err, err, problem)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty on malformed success data", stdout.String())
			}
		})
	}
}

func TestMeetingParticipantKickoutExecuteCorrelatesCanonicalServerIDWithoutRewritingEnvelope(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/meetings/7651377260537433044/kickout",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"kickout_results": []interface{}{
					map[string]interface{}{"id": int64(123), "user_type": 1, "result": 1},
				},
			},
		},
	})

	err := mountAndRun(t, VCMeetingParticipantKickout, []string{
		"+meeting-participant-kickout",
		"--meeting-id", "7651377260537433044",
		"--participant", "000123=1",
		"--yes", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": 123`) {
		t.Fatalf("stdout = %q, want untouched numeric server id", stdout.String())
	}
}

func TestBuildMeetingParticipantKickoutPathTrimsAndEscapesMeetingID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trim", in: " 7651377260537433044 ", want: "/open-apis/vc/v1/meetings/7651377260537433044/kickout"},
		{name: "escape segment", in: "1/2", want: "/open-apis/vc/v1/meetings/1%2F2/kickout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildMeetingParticipantKickoutPath(tt.in); got != tt.want {
				t.Fatalf("buildMeetingParticipantKickoutPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
