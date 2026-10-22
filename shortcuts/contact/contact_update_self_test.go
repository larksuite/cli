// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
)

// Each test owns its credential and transport dependencies through TestFactory.
// No test consults a developer profile or installs a process-wide provider.
type selfAvatarCredentials struct {
	config      *core.CliConfig
	scopes      string
	tokenValues int
	tokenError  error
}

func (p *selfAvatarCredentials) ResolveAccount(context.Context) (*credential.Account, error) {
	return credential.AccountFromCliConfig(p.config), nil
}
func (p *selfAvatarCredentials) ResolveToken(context.Context, credential.TokenSpec) (*credential.TokenResult, error) {
	if p.tokenError != nil {
		return nil, p.tokenError
	}
	p.tokenValues++
	return &credential.TokenResult{Token: "self-profile-fixture-token", Scopes: p.scopes}, nil
}

type selfAvatarAgentCredentials struct{ *selfAvatarCredentials }

func (*selfAvatarAgentCredentials) ResolveToken(context.Context, credential.TokenSpec) (*credential.TokenResult, error) {
	return nil, credential.AgentLegacyTokenError()
}

func (p *selfAvatarAgentCredentials) ResolveAccess(_ context.Context, acct *credential.Account, spec credential.AccessSpec) (*credential.ResolvedAccess, bool, error) {
	if spec.Identity != core.AsUser {
		return nil, true, credential.AgentBotIdentityError()
	}
	p.tokenValues++
	return &credential.ResolvedAccess{
		Value: "self-profile-fixture-token", Kind: core.CredentialAAT,
		OpenID: acct.PrincipalOpenID, ExpiresAt: time.Now().Add(time.Hour),
	}, true, nil
}

func useSelfAvatarAgent(wire *selfAvatarWire) {
	creds := wire.credentials
	creds.config.CredentialKind = core.CredentialAAT
	creds.config.PrincipalOpenID = "ou_self_fixture"
	creds.config.AgentScope = creds.scopes
	wire.factory.Credential = credential.NewCredentialProvider(nil, creds, &selfAvatarAgentCredentials{creds}, wire.factory.HttpClient)
}

type selfAvatarWireRequest struct {
	Method     string
	Query      url.Values
	Body       json.RawMessage
	ImageType  string
	ImageName  string
	ImageBytes []byte
}

type selfAvatarWire struct {
	factory        *cmdutil.Factory
	stdout, stderr *bytes.Buffer
	credentials    *selfAvatarCredentials
	requests       []selfAvatarWireRequest
	openID         string
	patchResponse  string
	uploadResponse string
}

func setupSelfAvatarWire(t *testing.T, scopes string) *selfAvatarWire {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	config := &core.CliConfig{
		AppID: "cli_self_profile_fixture", AppSecret: "test", Brand: core.BrandFeishu,
		UserOpenId: "ou_self_fixture", DefaultAs: core.AsUser,
	}
	factory, stdout, stderr, registry := cmdutil.TestFactory(t, config)
	creds := &selfAvatarCredentials{config: config, scopes: scopes}
	wire := &selfAvatarWire{
		factory: factory, stdout: stdout, stderr: stderr, credentials: creds,
		openID: "ou_self_fixture", patchResponse: `{"code":0,"data":{"employee_id":"ou_self_fixture"}}`,
	}
	factory.Credential = credential.NewCredentialProvider(nil, creds, creds, factory.HttpClient)
	register := func(method, path string, response func(selfAvatarWireRequest) any) {
		stub := &httpmock.Stub{Method: method, URL: core.ResolveOpenBaseURL(config.Brand) + path, Reusable: true, Optional: true,
			Headers: http.Header{"Content-Type": {"application/json"}, "X-Tt-Logid": {"self-profile-fixture-log"}}}
		stub.OnMatch = func(req *http.Request) {
			if req.URL.Path != path || req.Header.Get("Authorization") != "Bearer self-profile-fixture-token" {
				t.Fatalf("unexpected request path or fixture credential: %s %s", req.Method, req.URL.Path)
			}
			got := selfAvatarWireRequest{Method: req.Method, Query: req.URL.Query(), Body: stub.CapturedBody}
			if method == http.MethodPost {
				readSelfAvatarMultipart(t, req, &got)
			}
			wire.requests = append(wire.requests, got)
			stub.Body = response(got)
		}
		registry.Register(stub)
	}
	register(http.MethodGet, "/open-apis/authen/v1/user_info", func(selfAvatarWireRequest) any {
		return map[string]any{"code": 0, "data": map[string]string{"open_id": wire.openID}}
	})
	register(http.MethodPatch, "/open-apis/directory/v1/employees/ou_self_fixture", func(selfAvatarWireRequest) any { return wire.patchResponse })
	register(http.MethodPost, "/open-apis/im/v1/images", func(selfAvatarWireRequest) any {
		if wire.uploadResponse != "" {
			return wire.uploadResponse
		}
		return `{"code":0,"data":{"image_key":"img_fixture_avatar"}}`
	})
	// Fail even when a negative case expects an error: an unexpected request
	// must never masquerade as successful input validation.
	registry.Register(&httpmock.Stub{Reusable: true, Optional: true, OnMatch: func(req *http.Request) {
		t.Fatalf("unexpected HTTP request: %s %s", req.Method, req.URL.Path)
	}})
	return wire
}

func readSelfAvatarMultipart(t *testing.T, req *http.Request, got *selfAvatarWireRequest) {
	t.Helper()
	if err := req.ParseMultipartForm(11 << 20); err != nil {
		t.Fatalf("invalid multipart image upload: %v", err)
	}
	form := req.MultipartForm
	t.Cleanup(func() { _ = form.RemoveAll() })
	if !reflect.DeepEqual(form.Value, map[string][]string{"image_type": {"avatar"}}) || len(form.File) != 1 || len(form.File["image"]) != 1 {
		t.Fatalf("invalid avatar form: values=%v files=%v", form.Value, form.File)
	}
	header := form.File["image"][0]
	file, err := header.Open()
	if err != nil {
		t.Fatal(err)
	}
	got.ImageType, got.ImageName = "avatar", header.Filename
	got.ImageBytes, err = io.ReadAll(file)
	if closeErr := file.Close(); err != nil || closeErr != nil {
		t.Fatalf("read uploaded image: %v / %v", err, closeErr)
	}
}

type selfAvatarResult struct {
	EmployeeID     string   `json:"employee_id"`
	EmployeeIDType string   `json:"employee_id_type"`
	UpdatedFields  []string `json:"updated_fields"`
}

func executeSelfAvatar(t *testing.T, wire *selfAvatarWire, flags ...string) (selfAvatarResult, string, error) {
	t.Helper()
	args := append([]string{"+update-self", "--as", "user", "--format", "json"}, flags...)
	err := mountAndRunContext(t, context.Background(), ContactUpdateSelf, args, wire.factory, wire.stdout)
	if err != nil {
		return selfAvatarResult{}, wire.stdout.String(), err
	}
	var envelope struct {
		OK   bool             `json:"ok"`
		Data selfAvatarResult `json:"data"`
	}
	if err := json.Unmarshal(wire.stdout.Bytes(), &envelope); err != nil || !envelope.OK {
		t.Fatalf("invalid success envelope: %s err=%v stderr=%s", wire.stdout.String(), err, wire.stderr.String())
	}
	return envelope.Data, wire.stdout.String(), nil
}

func (w *selfAvatarWire) calls(method string) []selfAvatarWireRequest {
	var requests []selfAvatarWireRequest
	for _, req := range w.requests {
		if req.Method == method {
			requests = append(requests, req)
		}
	}
	return requests
}

func TestUpdateSelfRejectsNonAvatarFlags(t *testing.T) {
	for _, flag := range []string{"--name", "--alias", "--description", "--email", "--mobile", "--background-image-key", "--background-image-file"} {
		t.Run(flag, func(t *testing.T) {
			wire := setupSelfAvatarWire(t, "directory:employee.update:write im:resource")
			files := installSelfAvatarFiles(t, wire)
			_, output, err := executeSelfAvatar(t, wire, "--avatar-key", "img_fixture_avatar", flag, "unsupported")
			if err == nil || !strings.Contains(err.Error(), "unknown flag: "+flag) {
				t.Fatalf("non-avatar flag must be rejected by the parser: err=%v", err)
			}
			if output != "" || len(wire.requests) != 0 || wire.credentials.tokenValues != 0 || files.stats != 0 || files.opens != 0 {
				t.Fatalf("rejected input caused side effects: output=%s requests=%d tokens=%d stats=%d opens=%d", output, len(wire.requests), wire.credentials.tokenValues, files.stats, files.opens)
			}
		})
	}
}

func TestUpdateSelfAvatarKey(t *testing.T) {
	wire := setupSelfAvatarWire(t, "directory:employee.update:write")
	data, output, err := executeSelfAvatar(t, wire, "--avatar-key", "img_fixture_avatar")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	requests := wire.calls("PATCH")
	if len(wire.requests) != 2 || wire.requests[0].Method != "GET" || len(requests) != 1 {
		t.Fatalf("expected user_info followed by one PATCH: %+v", wire.requests)
	}
	var got map[string]map[string]string
	if err := json.Unmarshal(requests[0].Body, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]map[string]string{"employee": {"avatar_key": "img_fixture_avatar"}}) {
		t.Fatalf("PATCH must contain only the avatar: %s", requests[0].Body)
	}
	if !reflect.DeepEqual(requests[0].Query, url.Values{"employee_id_type": {"open_id"}, "user_auth_type": {"user"}}) {
		t.Fatalf("query=%v", requests[0].Query)
	}
	if data.EmployeeID != "ou_self_fixture" || data.EmployeeIDType != "open_id" || !reflect.DeepEqual(data.UpdatedFields, []string{"avatar_key"}) {
		t.Fatalf("data=%#v", data)
	}
	if strings.Contains(output, "img_fixture_avatar") || strings.Contains(output, "fixture-token") {
		t.Fatalf("success output exposed image key or token: %s", output)
	}
}

func TestUpdateSelfAgentUsesCurrentPrincipal(t *testing.T) {
	for _, file := range []bool{false, true} {
		t.Run(map[bool]string{false: "key", true: "file"}[file], func(t *testing.T) {
			wire := setupSelfAvatarWire(t, "directory:employee.update:write im:resource")
			useSelfAvatarAgent(wire)
			t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "on")
			flags := []string{"--avatar-key", "img_fixture_avatar"}
			wantRequests := 2
			if file {
				installSelfAvatarFiles(t, wire)
				flags, wantRequests = []string{"--avatar-file", "avatar.png"}, 3
			}
			data, _, err := executeSelfAvatar(t, wire, flags...)
			if err != nil || data.EmployeeID != "ou_self_fixture" || len(wire.requests) != wantRequests {
				t.Fatalf("AAT update failed: data=%+v requests=%+v err=%v", data, wire.requests, err)
			}
			patch := wire.calls("PATCH")[0]
			var body map[string]map[string]string
			if err := json.Unmarshal(patch.Body, &body); err != nil ||
				!reflect.DeepEqual(body, map[string]map[string]string{"employee": {"avatar_key": "img_fixture_avatar"}}) ||
				!reflect.DeepEqual(patch.Query, url.Values{"employee_id_type": {"open_id"}, "user_auth_type": {"user"}}) {
				t.Fatalf("AAT must update only its own avatar on the user lane: %+v err=%v", patch, err)
			}
		})
	}
}

func TestUpdateSelfRequiresUpdateScope(t *testing.T) {
	wire := setupSelfAvatarWire(t, "im:resource")
	_, output, err := executeSelfAvatar(t, wire, "--avatar-key", "img_fixture_avatar")
	var permission *errs.PermissionError
	if !errors.As(err, &permission) || permission.Subtype != errs.SubtypeMissingScope ||
		!reflect.DeepEqual(permission.MissingScopes, []string{"directory:employee.update:write"}) ||
		len(wire.requests) != 0 || output != "" {
		t.Fatalf("missing update grant must stop before requests: err=%v output=%s requests=%+v", err, output, wire.requests)
	}
}

func TestUpdateSelfRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		flags []string
		param string
	}{
		{nil, ""},
		{[]string{"--avatar-key", ""}, "--avatar-key"},
		{[]string{"--avatar-key", " "}, "--avatar-key"},
		{[]string{"--avatar-file", "https://example.com/avatar.png"}, "--avatar-file"},
		{[]string{"--avatar-file", ""}, "--avatar-file"},
		{[]string{"--avatar-file", "avatar.png", "--avatar-key", "img_existing"}, "--avatar-file"},
		{[]string{"--avatar-key", "img_existing", "--as", "bot"}, ""},
		{[]string{"--avatar-key", "img_existing", "--employee-id", "ou_other"}, ""},
		{[]string{"--avatar-key", "img_existing", "--user-auth-type", "admin_user"}, ""},
		{[]string{"--avatar-key", "img_existing", "--data", `{}`}, ""},
	} {
		t.Run(strings.Join(tc.flags, " "), func(t *testing.T) {
			wire := setupSelfAvatarWire(t, "directory:employee.update:write im:resource")
			files := installSelfAvatarFiles(t, wire)
			_, output, err := executeSelfAvatar(t, wire, tc.flags...)
			if err == nil || output != "" || len(wire.requests) != 0 || files.stats != 0 || files.opens != 0 {
				t.Fatalf("err=%v output=%s requests=%d stat=%d open=%d", err, output, len(wire.requests), files.stats, files.opens)
			}
			var validation *errs.ValidationError
			if tc.param != "" && (!errors.As(err, &validation) || validation.Param != tc.param) {
				t.Fatalf("err=%v validation=%+v want param=%s", err, validation, tc.param)
			}
		})
	}
}

func TestUpdateSelfDryRunIsOffline(t *testing.T) {
	wire := setupSelfAvatarWire(t, "")
	_, output, err := executeSelfAvatar(t, wire, "--avatar-key", "img_preview", "--dry-run")
	if err != nil || len(wire.requests) != 0 || wire.credentials.tokenValues != 0 {
		t.Fatalf("dry-run err=%v requests=%+v token values=%d", err, wire.requests, wire.credentials.tokenValues)
	}
	if !strings.Contains(output, "user_auth_type") || !strings.Contains(output, "open_id") || !strings.Contains(output, "preview") {
		t.Fatalf("dry-run lacks intended profile update: %s", output)
	}
}

func TestUpdateSelfRejectsMissingResolvedUser(t *testing.T) {
	for _, id := range []string{"", " "} {
		t.Run(id, func(t *testing.T) {
			wire := setupSelfAvatarWire(t, "directory:employee.update:write")
			wire.openID = id
			_, output, err := executeSelfAvatar(t, wire, "--avatar-key", "img_fixture_avatar")
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Subtype != errs.SubtypeInvalidResponse || output != "" || len(wire.calls("PATCH")) != 0 {
				t.Fatalf("err=%v output=%s patches=%d", err, output, len(wire.calls("PATCH")))
			}
		})
	}
}
