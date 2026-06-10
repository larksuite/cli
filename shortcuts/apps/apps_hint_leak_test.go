// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"regexp"
	"testing"
)

// TestAppsErrorHintsCarryNoSecretsOrPII guards the actionable error hints added
// for the apps command-governance task. Those hints are inline string literals
// spread across several files (apps_env_pull.go, apps_access_scope_set.go,
// apps_access_scope_get.go, apps_init.go git-push path, and the
// gitCredentialIssueHint const in git_credential.go). They are stable English
// strings, so we assert the verbatim copies here: a real app_id, an email, or a
// phone number must never appear in a hint. Placeholders like <app_id> are
// expected and must NOT trip the real-app-id regex.
func TestAppsErrorHintsCarryNoSecretsOrPII(t *testing.T) {
	// These are copied verbatim from the source. If a hint changes, copy the new
	// text here so this leak guard keeps tracking the real production string.
	hints := []string{
		// apps_env_pull.go:86 and apps_access_scope_get.go:50 (identical literals)
		"verify --app-id is correct and you have access to the app; list your apps with `lark-cli apps +list`",
		// apps_access_scope_set.go:74
		"verify --app-id is correct; for scope=specific, each --targets id must be a valid open_id/department_id/chat_id and --approver a valid open_id; review the current scope with `lark-cli apps +access-scope-get --app-id <app_id>`",
		// apps_init.go:483 (git push rejection)
		"the push was rejected — the git output is in the message above; if it is a non-fast-forward (remote has new commits), sync the remote and retry; if it is an auth failure, make sure `lark-cli apps +git-credential-init` has succeeded",
		// git_credential.go gitCredentialIssueHint const (referenced directly so a
		// rename or text change breaks the build instead of silently drifting)
		gitCredentialIssueHint,
		// command-governance hints added for this task (referenced by const, no drift)
		appIDListHint,
		sessionStopHint,
		createHint,
		dbEnvCreateHint,
		dbTableGetHint,
		dbTableListHint,
	}

	realAppID := regexp.MustCompile(`app_[a-z0-9]{6,}`)
	email := regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phone := regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	// An obvious secret: a PAT-like token or a "secret=..." / "token=..." pair.
	secret := regexp.MustCompile(`(?i)(pat-[a-z0-9]+|secret\s*[=:]\s*\S|token\s*[=:]\s*\S)`)

	for _, h := range hints {
		if realAppID.MatchString(h) {
			t.Errorf("hint leaks a real-looking app id (use <app_id>): %q", h)
		}
		if email.MatchString(h) {
			t.Errorf("hint leaks an email address: %q", h)
		}
		if phone.MatchString(h) {
			t.Errorf("hint leaks a phone number: %q", h)
		}
		if secret.MatchString(h) {
			t.Errorf("hint leaks an obvious secret/token: %q", h)
		}
	}

	// Sanity: the placeholder <app_id> must NOT match the real-app-id regex,
	// otherwise the guard above would be a false positive on legitimate hints.
	if realAppID.MatchString("<app_id>") {
		t.Fatal("realAppID regex incorrectly matches the <app_id> placeholder")
	}
}
