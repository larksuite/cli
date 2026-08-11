// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// Tests pinning bot-identity support for the vc read shortcuts
// (+detail / +notes / +recording).

package vc

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/credential"
)

// ---------------------------------------------------------------------------
// AuthTypes contracts
// ---------------------------------------------------------------------------

func TestVCReadShortcutsSupportUserAndBotIdentity(t *testing.T) {
	want := []string{"user", "bot"}
	cases := map[string][]string{
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
// so an under-scoped UAT must not make --as bot fail. Reverting to
// auth.GetStoredToken(user) would break this.
// ---------------------------------------------------------------------------

func TestRecording_BotIdentityAwareScopePreflight(t *testing.T) {
	cfg := defaultConfig()
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	f.Credential = credential.NewCredentialProvider(nil, nil, &recordingIdentityTokenResolver{
		uatScopes: "calendar:calendar:read", // deliberately missing vc:record:readonly
		tatScopes: "",                       // bot/tenant: no local scope metadata
	}, nil)

	err := mountAndRun(t, VCRecording, []string{"+recording", "--meeting-ids", "m001", "--dry-run", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("bot preflight must resolve tenant token, not the under-scoped user token; got error: %v", err)
	}
}

// recordingIdentityTokenResolver returns different scopes for UAT vs TAT so
// bot identity-aware preflight can be pinned separately from user preflight.
type recordingIdentityTokenResolver struct {
	uatScopes string
	tatScopes string
}

func (r *recordingIdentityTokenResolver) ResolveToken(_ context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	scopes := r.uatScopes
	if req.Type == credential.TokenTypeTAT {
		scopes = r.tatScopes
	}
	return &credential.TokenResult{Token: "test-token", Scopes: scopes}, nil
}
