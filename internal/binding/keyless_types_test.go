// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import "testing"

func TestListCandidateApps_KeylessSingleAccount(t *testing.T) {
	apps := ListCandidateApps(&FeishuChannel{
		AppID:      "cli_keyless",
		Brand:      "feishu",
		AuthMethod: AuthMethodPrivateKeyJWT,
		KeyRef:     "openclaw-lark",
	})
	if len(apps) != 1 {
		t.Fatalf("count = %d, want 1", len(apps))
	}
	if !apps[0].IsKeyless() {
		t.Fatalf("candidate = %#v, want keyless", apps[0])
	}
	if apps[0].KeyRef != "openclaw-lark" {
		t.Fatalf("KeyRef = %q, want openclaw-lark", apps[0].KeyRef)
	}
}

func TestListCandidateApps_KeylessMultiAccountInheritance(t *testing.T) {
	apps := ListCandidateApps(&FeishuChannel{
		AppID:      "cli_top",
		Brand:      "lark",
		AuthMethod: AuthMethodPrivateKeyJWT,
		KeyRef:     "openclaw-lark",
		Accounts: map[string]*FeishuAccount{
			"work": {},
		},
	})
	if len(apps) != 2 {
		t.Fatalf("count = %d, want 2 (implicit default + work)", len(apps))
	}
	app := candidateByLabel(t, apps, "work")
	if app.Label != "work" || app.AppID != "cli_top" || app.Brand != "lark" {
		t.Fatalf("candidate identity = %#v", app)
	}
	if app.AuthMethod != AuthMethodPrivateKeyJWT || app.KeyRef != "openclaw-lark" || !app.IsKeyless() {
		t.Fatalf("candidate keyless fields = %#v", app)
	}
}

func TestListCandidateApps_KeylessAccountOverride(t *testing.T) {
	apps := ListCandidateApps(&FeishuChannel{
		AppID:      "cli_top",
		AuthMethod: AuthMethodPrivateKeyJWT,
		KeyRef:     "top-key",
		Accounts: map[string]*FeishuAccount{
			"work": {AppID: "cli_work", KeyRef: "work-key"},
		},
	})
	if len(apps) != 2 {
		t.Fatalf("count = %d, want 2 (implicit default + work)", len(apps))
	}
	if got := candidateByLabel(t, apps, "work"); got.AppID != "cli_work" || got.KeyRef != "work-key" || !got.IsKeyless() {
		t.Fatalf("candidate = %#v", got)
	}
}

func candidateByLabel(t *testing.T, apps []CandidateApp, label string) CandidateApp {
	t.Helper()
	for _, app := range apps {
		if app.Label == label {
			return app
		}
	}
	t.Fatalf("candidate %q not found in %#v", label, apps)
	return CandidateApp{}
}

func TestCandidateApp_SecretTakesPrecedenceOverKeyless(t *testing.T) {
	app := CandidateApp{
		AppID:      "cli_both",
		AppSecret:  SecretInput{Plain: "secret"},
		AuthMethod: AuthMethodPrivateKeyJWT,
		KeyRef:     "openclaw-lark",
	}
	if app.IsKeyless() {
		t.Fatal("an appSecret-backed OpenClaw account must not be treated as keyless")
	}
}

func TestCandidateApp_KeylessRequiresKeyRef(t *testing.T) {
	app := CandidateApp{AppID: "cli_missing_key", AuthMethod: AuthMethodPrivateKeyJWT}
	if app.IsKeyless() {
		t.Fatal("private_key_jwt without keyRef must not be treated as usable keyless")
	}
}

func TestListCandidateApps_KeylessImplicitDefault(t *testing.T) {
	apps := ListCandidateApps(&FeishuChannel{
		AppID:      "cli_default",
		AuthMethod: AuthMethodPrivateKeyJWT,
		KeyRef:     "openclaw-lark",
		Accounts: map[string]*FeishuAccount{
			"work": {AppID: "cli_work", AuthMethod: AuthMethodPrivateKeyJWT, KeyRef: "work-key"},
		},
	})
	if len(apps) != 2 {
		t.Fatalf("count = %d, want 2", len(apps))
	}
	for _, app := range apps {
		if !app.IsKeyless() {
			t.Fatalf("candidate = %#v, want keyless", app)
		}
	}
}
