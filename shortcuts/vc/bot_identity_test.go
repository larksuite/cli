// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// Tests pinning bot-identity support for the vc read shortcuts
// (+search / +detail / +notes / +recording).

package vc

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
)

// ---------------------------------------------------------------------------
// AuthTypes contracts
// ---------------------------------------------------------------------------

func TestVCReadShortcutsSupportUserAndBotIdentity(t *testing.T) {
	want := []string{"user", "bot"}
	cases := map[string][]string{
		"+search":    VCSearch.AuthTypes,
		"+detail":    VCDetail.AuthTypes,
		"+notes":     VCNotes.AuthTypes,
		"+recording": VCRecording.AuthTypes,
	}
	for cmd, got := range cases {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s AuthTypes = %v, want %v", cmd, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Bot dry-run: the meeting/recording paths flow under bot identity
// ---------------------------------------------------------------------------

func TestDetail_DryRun_BotIdentity(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCDetail, []string{"+detail", "--meeting-ids", "m001", "--dry-run", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error under --as bot: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/vc/v1/meetings/{meeting_id}") {
		t.Errorf("dry-run should show meeting.get API, got: %s", out)
	}
	if !strings.Contains(out, "recording") {
		t.Errorf("dry-run should show recording API, got: %s", out)
	}
}

func TestRecording_DryRun_BotIdentity(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCRecording, []string{"+recording", "--meeting-ids", "m001", "--dry-run", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error under --as bot: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "recording") {
		t.Errorf("dry-run should show recording API, got: %s", out)
	}
}

func TestNotes_DryRun_BotIdentity(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m001", "--dry-run", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error under --as bot: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/vc/v1/notes/{note_id}") {
		t.Errorf("dry-run should show note.get API, got: %s", out)
	}
}

// ---------------------------------------------------------------------------
// calendar-event-ids also flows under bot: a bot has a primary calendar, so the
// primary-calendar -> meeting_id -> recording/notes chain is expected to work.
// ---------------------------------------------------------------------------

func TestRecording_DryRun_BotIdentity_CalendarEventIDs(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCRecording, []string{"+recording", "--calendar-event-ids", "evt001", "--dry-run", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error under --as bot: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "mget_instance_relation_info") {
		t.Errorf("dry-run should show the primary-calendar resolution step, got: %s", out)
	}
	if !strings.Contains(out, "recording") {
		t.Errorf("dry-run should show recording API, got: %s", out)
	}
}

func TestNotes_DryRun_BotIdentity_CalendarEventIDs(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCNotes, []string{"+notes", "--calendar-event-ids", "evt001", "--dry-run", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error under --as bot: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "mget_instance_relation_info") {
		t.Errorf("dry-run should show the primary-calendar resolution step, got: %s", out)
	}
}

// ---------------------------------------------------------------------------
// Identity-aware preflight: bot resolves TAT (empty local scopes in this stub),
// so an under-scoped UAT must not make --as bot fail. The req.Type assertions
// below pin that the credential layer is asked for a tenant token (not a user
// token) when --as bot is used — this holds regardless of which layer issues
// the request (the shared per-shortcut scope preflight or vc_recording.go's
// own calendar-event-ids check), so it does not by itself catch a revert of
// vc_recording.go's Validate. TestRecording_CalendarEventIDs_MissingExtraScope
// below is the test that actually fails if that shortcut-local check regresses.
// ---------------------------------------------------------------------------

func TestSearch_BotIdentityResolvesTenantToken(t *testing.T) {
	cfg := defaultConfig()
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	resolver := &recordingIdentityTokenResolver{tatScopes: ""}
	f.Credential = credential.NewCredentialProvider(nil, nil, resolver, nil)

	err := mountAndRun(t, VCSearch, []string{
		"+search", "--query", "weekly", "--page-size", "5",
		"--page-token", "next", "--dry-run", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected bot dry-run error: %v", err)
	}
	if len(resolver.requestsOfType(credential.TokenTypeTAT)) == 0 {
		t.Fatalf("expected bot search to resolve TAT, requests: %v", resolver.requests)
	}
	if got := resolver.requestsOfType(credential.TokenTypeUAT); len(got) != 0 {
		t.Fatalf("bot search must not resolve UAT, requests: %v", got)
	}
}

func TestSearch_BotPermissionErrorKeepsIdentityAndScope(t *testing.T) {
	cfg := defaultConfig()
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	resolver := &recordingIdentityTokenResolver{tatScopes: ""}
	f.Credential = credential.NewCredentialProvider(nil, nil, resolver, nil)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/meetings/search",
		Body: map[string]interface{}{
			"code": 99991672,
			"msg":  "app scope not enabled",
			"error": map[string]interface{}{
				"permission_violations": []interface{}{
					map[string]interface{}{"subject": "vc:meeting.search:read"},
				},
			},
		},
	})

	err := mountAndRun(t, VCSearch, []string{
		"+search", "--query", "weekly", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected bot permission error")
	}
	var permissionErr *errs.PermissionError
	if !errors.As(err, &permissionErr) {
		t.Fatalf("expected *errs.PermissionError, got %T: %v", err, err)
	}
	if permissionErr.Code != 99991672 || permissionErr.Identity != "bot" {
		t.Fatalf("permission error = %+v, want code 99991672 and bot identity", permissionErr)
	}
	if !slices.Contains(permissionErr.MissingScopes, "vc:meeting.search:read") {
		t.Fatalf("missing scopes = %v, want vc:meeting.search:read", permissionErr.MissingScopes)
	}
	if strings.Contains(permissionErr.Hint, "auth login") {
		t.Fatalf("bot permission hint must not suggest user login: %q", permissionErr.Hint)
	}
	reg.Verify(t)
}

func TestRecording_BotIdentityAwareScopePreflight(t *testing.T) {
	cfg := defaultConfig()
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	resolver := &recordingIdentityTokenResolver{
		uatScopes: "calendar:calendar:read", // deliberately missing vc:record:readonly
		tatScopes: "",                       // bot/tenant: no local scope metadata
	}
	f.Credential = credential.NewCredentialProvider(nil, nil, resolver, nil)

	err := mountAndRun(t, VCRecording, []string{"+recording", "--meeting-ids", "m001", "--dry-run", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("bot preflight must resolve tenant token, not the under-scoped user token; got error: %v", err)
	}

	reqs := resolver.requestsOfType(credential.TokenTypeTAT)
	if len(reqs) == 0 {
		t.Fatalf("expected ResolveToken to be called with TokenTypeTAT for --as bot, got requests: %v", resolver.requests)
	}
	for _, req := range reqs {
		if req.AppID != cfg.AppID {
			t.Errorf("TAT request AppID = %q, want %q", req.AppID, cfg.AppID)
		}
	}
	if uatReqs := resolver.requestsOfType(credential.TokenTypeUAT); len(uatReqs) != 0 {
		t.Errorf("--as bot must not resolve a UAT, got: %v", uatReqs)
	}
}

// TestRecording_UserIdentityScopePreflight is the user-identity counterpart:
// --as user must resolve a UAT, never a TAT.
func TestRecording_UserIdentityScopePreflight(t *testing.T) {
	cfg := defaultConfig()
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	resolver := &recordingIdentityTokenResolver{
		uatScopes: "vc:record:readonly",
		tatScopes: "",
	}
	f.Credential = credential.NewCredentialProvider(nil, nil, resolver, nil)

	err := mountAndRun(t, VCRecording, []string{"+recording", "--meeting-ids", "m001", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error under --as user: %v", err)
	}

	if reqs := resolver.requestsOfType(credential.TokenTypeUAT); len(reqs) == 0 {
		t.Fatalf("expected ResolveToken to be called with TokenTypeUAT for --as user, got requests: %v", resolver.requests)
	}
	if reqs := resolver.requestsOfType(credential.TokenTypeTAT); len(reqs) != 0 {
		t.Errorf("--as user must not resolve a TAT, got: %v", reqs)
	}
}

// TestRecording_CalendarEventIDs_MissingExtraScope pins the
// calendar-event-ids-only scope preflight in vc_recording.go's Validate: the
// generic per-shortcut scope check only knows about the statically declared
// vc:record:readonly, so the calendar read scopes required for the
// calendar-event-ids resolution path are only caught by this shortcut-local
// check. Reverting it to the old auth.GetStoredToken(user)-only lookup, or
// dropping the calendar-event-ids branch, would let this under-scoped
// request reach the API undetected.
func TestRecording_CalendarEventIDs_MissingExtraScope(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	f.Credential = credential.NewCredentialProvider(nil, nil, &recordingScopedTokenResolver{
		scopes: "vc:record:readonly", // satisfies the shortcut's static Scopes, but not the calendar ones
	}, nil)

	err := mountAndRun(t, VCRecording, []string{"+recording", "--calendar-event-ids", "evt001", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected missing_scope error for calendar-event-ids path, got nil")
	}
	var pe *errs.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *errs.PermissionError, got %T: %v", err, err)
	}
	if pe.Subtype != errs.SubtypeMissingScope {
		t.Errorf("Subtype = %q, want %q", pe.Subtype, errs.SubtypeMissingScope)
	}
	for _, want := range []string{"calendar:calendar:read", "calendar:calendar.event:read"} {
		found := false
		for _, s := range pe.MissingScopes {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("MissingScopes = %v, want to contain %q", pe.MissingScopes, want)
		}
	}
}

// TestRecording_ScopePreflight_ResolveTokenError pins the fail-open behavior
// when the credential layer cannot resolve a token at all: the shortcut-local
// preflight must not block the request (the API call itself remains the
// authoritative check), so dry-run still succeeds.
func TestRecording_ScopePreflight_ResolveTokenError(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	f.Credential = credential.NewCredentialProvider(nil, nil, &erroringTokenResolver{}, nil)

	err := mountAndRun(t, VCRecording, []string{"+recording", "--meeting-ids", "m001", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("a token-resolution error must not block the request, got: %v", err)
	}
}

// recordingIdentityTokenResolver returns different scopes for UAT vs TAT so
// bot identity-aware preflight can be pinned separately from user preflight,
// and records every request it receives so tests can assert on TokenSpec.Type.
type recordingIdentityTokenResolver struct {
	uatScopes string
	tatScopes string
	requests  []credential.TokenSpec
}

func (r *recordingIdentityTokenResolver) ResolveToken(_ context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	r.requests = append(r.requests, req)
	scopes := r.uatScopes
	if req.Type == credential.TokenTypeTAT {
		scopes = r.tatScopes
	}
	return &credential.TokenResult{Token: "test-token", Scopes: scopes}, nil
}

func (r *recordingIdentityTokenResolver) requestsOfType(t credential.TokenType) []credential.TokenSpec {
	var out []credential.TokenSpec
	for _, req := range r.requests {
		if req.Type == t {
			out = append(out, req)
		}
	}
	return out
}

// erroringTokenResolver simulates a credential layer that cannot resolve any
// token (e.g. network failure resolving app credentials).
type erroringTokenResolver struct{}

func (r *erroringTokenResolver) ResolveToken(_ context.Context, _ credential.TokenSpec) (*credential.TokenResult, error) {
	return nil, errors.New("simulated credential resolution failure")
}
