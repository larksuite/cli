// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

// Legacy upgrade workflow regression: pre-multi-user lark-cli let
// `auth login` silently REPLACE the single stored user. Operators
// scripted that workflow ("logout, login as the next person, run
// stuff") for years.
//
// After the multi-user port introduced verifyHolder, a fresh
// authorization whose upstream open_id disagreed with
// AppConfig.CurrentUser was treated identically to a --user / env
// mismatch — a hard abort with SubtypeInvalidArgument. That broke
// the legacy workflow with NO security benefit: the operator did
// not declare any explicit target user, the only signal of intent
// was the stale CurrentUser left over from a prior login.
//
// The fix splits verifyHolder by holderSource:
//   - "flag" / "env" stay hard aborts (operator declared an explicit
//     target — typo / phishing / redirect guard).
//   - "" (implied via AppConfig.CurrentUser) is now a soft advisory:
//     login proceeds, the new user is appended to Users[], the
//     active user (CurrentUser) does not silently switch, and a
//     stderr WARN tells the operator how to switch via
//     `auth users use`.
//
// This test pins down the legacy-workflow contract:
//
//	Pre-state:  Users=[Alice], CurrentUser=Alice, no --user / env
//	Action:     `auth login`, upstream returns Bob's open_id
//	Post-state: Users=[Alice, Bob], CurrentUser=Alice, advisory in stderr
//	Exit:       0 (success)
func TestAuthLoginRun_LegacyReLoginWorkflow_AdvisoryNotAbort(t *testing.T) {
	keyring.MockInit()
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:        "default",
			AppId:       "cli_test",
			CurrentUser: "ou_alice",
			Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	})
	f, _, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	stubLoginHTTP(t, reg, "ou_bob", "Bob")

	// Critical: NO --user, NO LARKSUITE_CLI_OPEN_ID. The implied holder
	// is AppConfig.CurrentUser=ou_alice. Upstream returns ou_bob.
	// Pre-fix this combo aborted hard. Post-fix it must proceed with
	// an advisory.
	f.Invocation = cmdutil.InvocationContext{Profile: "default"}

	if err := authLoginRun(&LoginOptions{
		Factory: f, Ctx: context.Background(), Scope: "im:message:send",
	}); err != nil {
		t.Fatalf("legacy re-login workflow regressed — got abort error: %v", err)
	}

	// Bob must be appended; Alice must remain; CurrentUser must not switch.
	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig: %v", err)
	}
	users := saved.Apps[0].Users
	if len(users) != 2 {
		t.Fatalf("Users len = %d, want 2 (Alice preserved + Bob appended); got %#v", len(users), users)
	}
	if users[0].UserOpenId != "ou_alice" || users[1].UserOpenId != "ou_bob" {
		t.Errorf("Users = [%q,%q], want [ou_alice,ou_bob]", users[0].UserOpenId, users[1].UserOpenId)
	}
	if saved.Apps[0].CurrentUser != "ou_alice" {
		t.Errorf("CurrentUser = %q, want ou_alice (must NOT silently switch)", saved.Apps[0].CurrentUser)
	}
	// Bob's token must have landed (login actually proceeded).
	if got := larkauth.GetStoredToken("cli_test", "ou_bob"); got == nil {
		t.Error("Bob's token slot is empty — login did not actually proceed")
	}

	// Advisory must reach stderr and contain the user-facing contract:
	// names both users, says "active user stays", points at users use,
	// AND uses the brand prefix every other lark-cli WARN line uses
	// (operators grep stderr for `[lark-cli]` to filter CI logs).
	stderrStr := stderr.String()
	wantSubs := []string{
		"[lark-cli]",
		"WARN",
		"auth login",
		"ou_alice",
		"ou_bob",
		"active user",
		"auth users use",
	}
	for _, sub := range wantSubs {
		if !strings.Contains(stderrStr, sub) {
			t.Errorf("stderr missing %q\nfull stderr:\n%s", sub, stderrStr)
		}
	}
	// The advisory MUST NOT contradict itself — operator did not pass --user.
	if strings.Contains(stderrStr, "re-run without") {
		t.Errorf("advisory must not suggest 're-run without' (operator did not pass --user); stderr:\n%s", stderrStr)
	}
}

// Counter-test: an EXPLICIT --user mismatch (not the legacy workflow)
// must STILL abort. The split is by holderSource, not just "is there
// a CurrentUser at all" — the soft path is intentionally narrow.
func TestAuthLoginRun_ExplicitFlagMismatch_StillAborts(t *testing.T) {
	keyring.MockInit()
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:        "default",
			AppId:       "cli_test",
			CurrentUser: "ou_alice",
			Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	})
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	stubLoginHTTP(t, reg, "ou_actually_bob", "Bob")

	// Operator typed --user ou_alice, but the device authorized ou_bob.
	// This is a typo / phishing guard — must hard-abort regardless of the
	// soft-mismatch carve-out for implied holders.
	f.Invocation = cmdutil.InvocationContext{
		Profile: "default", UserOpenId: "ou_alice", UserSource: "flag",
	}

	err := authLoginRun(&LoginOptions{
		Factory: f, Ctx: context.Background(), Scope: "im:message:send",
	})
	if err == nil {
		t.Fatal("expected hard abort on flag-source mismatch")
	}
	if !strings.Contains(err.Error(), "login holder mismatch") {
		t.Errorf("expected 'login holder mismatch' error, got: %v", err)
	}
	// Post-state: nothing persisted for the upstream user (pre-write abort).
	if got := larkauth.GetStoredToken("cli_test", "ou_actually_bob"); got != nil {
		t.Errorf("ou_actually_bob token persisted despite flag-mismatch abort: %#v", got)
	}
}

// Counter-test: an EXPLICIT env-source mismatch (LARKSUITE_CLI_OPEN_ID
// set in the environment) must ALSO hard-abort. Env-source is the primary
// CI phishing/redirect-guard scenario — operator runs `auth login` in CI
// with the env var pinning who the upstream identity must be, attacker-
// controlled device authorizes a different open_id. A regression that
// silently downgrades env-source to advisory would leak a fresh token
// into the wrong user slot.
//
// The unit-level TestVerifyHolder_EnvMismatch_EnvAttribution proves the
// abort branch in verifyHolder; this test proves the same outcome at the
// integration level by routing through enforceLoginHolderGate inside
// authLoginRun.
func TestAuthLoginRun_ExplicitEnvMismatch_StillAborts(t *testing.T) {
	keyring.MockInit()
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:        "default",
			AppId:       "cli_test",
			CurrentUser: "ou_alice",
			Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	})
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	stubLoginHTTP(t, reg, "ou_actually_bob", "Bob")

	// Operator's env had LARKSUITE_CLI_OPEN_ID=ou_alice; the device
	// authorized ou_actually_bob. UserSource: "env" is what the bootstrap
	// resolver stamps for env-sourced overrides.
	f.Invocation = cmdutil.InvocationContext{
		Profile: "default", UserOpenId: "ou_alice", UserSource: "env",
	}

	err := authLoginRun(&LoginOptions{
		Factory: f, Ctx: context.Background(), Scope: "im:message:send",
	})
	if err == nil {
		t.Fatal("expected hard abort on env-source mismatch")
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError on env-source mismatch, got %T: %v", err, err)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want SubtypeInvalidArgument", cfgErr.Subtype)
	}
	// The hint must name the env var so the operator knows which knob to
	// fix; otherwise they'll think it's a --user issue and waste time.
	if !strings.Contains(cfgErr.Hint, "LARKSUITE_CLI_OPEN_ID") {
		t.Errorf("env-source hint must mention LARKSUITE_CLI_OPEN_ID, got: %q", cfgErr.Hint)
	}
	// Post-state: the upstream user's token must NOT have been persisted —
	// the gate runs BEFORE SetStoredToken.
	if got := larkauth.GetStoredToken("cli_test", "ou_actually_bob"); got != nil {
		t.Errorf("ou_actually_bob token persisted despite env-mismatch abort: %#v", got)
	}
	// And no row in Users[].
	saved, _ := core.LoadMultiAppConfig()
	for _, u := range saved.Apps[0].Users {
		if u.UserOpenId == "ou_actually_bob" {
			t.Errorf("config grew an ou_actually_bob row despite env-source mismatch: %#v", u)
		}
	}
}

// Poll-path coverage: --no-wait + --device-code is the documented
// workflow for headless / SSH agents, and authLoginPollDeviceCode runs
// the same holder gate as the immediate path. If a refactor drops or
// drifts the gate at the second site, the legacy-workflow regression
// returns silently for the headless workflow with no integration test
// catching it. The gate now lives in enforceLoginHolderGate so the two
// sites cannot drift, but pin the contract end-to-end on the poll path
// too — defense in depth.
func TestAuthLoginPollDeviceCode_LegacyReLoginWorkflow_AdvisoryNotAbort(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	t.Setenv("HOME", t.TempDir())

	// Pre-state: Alice in CurrentUser. Bob authorizes via the device flow,
	// but the operator did NOT pass --user — implied holder is ou_alice,
	// upstream is ou_bob. Pre-fix the poll-path gate aborted; post-fix it
	// must emit the advisory and proceed.
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:        "default",
			AppId:       "cli_test",
			CurrentUser: "ou_alice",
			Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	f, _, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	// The poll path hits ONLY the OAuth token endpoint (device-code grant)
	// and the user-info endpoint — never device-authorization. Register
	// just those two; the registry's Verify() check fails on unmatched
	// stubs, so registering device-authorization here would falsely fail.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathOAuthTokenV2,
		Body: map[string]interface{}{
			"access_token":             "user-access-token",
			"refresh_token":            "refresh-token",
			"expires_in":               7200,
			"refresh_token_expires_in": 604800,
			"scope":                    "im:message:send offline_access",
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    larkauth.PathUserInfoV1,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"open_id":  "ou_bob",
				"union_id": "uid_ou_bob",
				"name":     "Bob",
			},
		},
	})

	// Cache the requested-scope record the way --no-wait would have.
	if err := saveLoginRequestedScope("device-code", "im:message:send"); err != nil {
		t.Fatalf("saveLoginRequestedScope: %v", err)
	}

	// Critical: NO --user, NO env. Implied holder is AppConfig.CurrentUser=ou_alice.
	f.Invocation = cmdutil.InvocationContext{Profile: "default"}

	if err := authLoginRun(&LoginOptions{
		Factory:    f,
		Ctx:        context.Background(),
		DeviceCode: "device-code",
	}); err != nil {
		t.Fatalf("poll-path legacy re-login regressed — got abort error: %v", err)
	}

	// Same post-state contract as the immediate-path test:
	saved, _ := core.LoadMultiAppConfig()
	users := saved.Apps[0].Users
	if len(users) != 2 {
		t.Fatalf("Users len = %d, want 2 (Alice preserved + Bob appended); got %#v", len(users), users)
	}
	if users[0].UserOpenId != "ou_alice" || users[1].UserOpenId != "ou_bob" {
		t.Errorf("Users = [%q,%q], want [ou_alice,ou_bob]", users[0].UserOpenId, users[1].UserOpenId)
	}
	if saved.Apps[0].CurrentUser != "ou_alice" {
		t.Errorf("CurrentUser = %q, want ou_alice (poll path must not silently switch)", saved.Apps[0].CurrentUser)
	}
	if got := larkauth.GetStoredToken("cli_test", "ou_bob"); got == nil {
		t.Error("Bob's token slot is empty — poll-path login did not actually proceed")
	}
	stderrStr := stderr.String()
	for _, sub := range []string{"[lark-cli]", "WARN", "ou_alice", "ou_bob", "active user", "auth users use"} {
		if !strings.Contains(stderrStr, sub) {
			t.Errorf("poll-path stderr missing %q\nfull stderr:\n%s", sub, stderrStr)
		}
	}
}

// JSON-mode integration coverage for S1: a legacy-workflow re-login on a
// JSON invocation must surface the `holder_mismatch_warning` structured
// field on the success payload AND keep emitting the human-readable WARN
// to stderr. The two channels are independent — JSON consumers branch on
// the structured field; humans tailing 2>&1 still see the same advisory.
func TestAuthLoginRun_LegacyReLoginWorkflow_JSONIncludesHolderMismatchWarning(t *testing.T) {
	keyring.MockInit()
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:        "default",
			AppId:       "cli_test",
			CurrentUser: "ou_alice",
			Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	})
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	stubLoginHTTP(t, reg, "ou_bob", "Bob")
	f.Invocation = cmdutil.InvocationContext{Profile: "default"}

	if err := authLoginRun(&LoginOptions{
		Factory: f, Ctx: context.Background(), Scope: "im:message:send", JSON: true,
	}); err != nil {
		t.Fatalf("legacy re-login regressed in JSON mode: %v", err)
	}

	// JSON mode emits NDJSON: the device_authorization line, then the
	// authorization_complete line. The structured holder warning rides on
	// the latter, so parse the last non-empty line.
	payload := lastJSONLine(t, stdout.String())
	raw, ok := payload["holder_mismatch_warning"]
	if !ok {
		t.Fatalf("payload missing holder_mismatch_warning; payload=%v", payload)
	}
	got, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("holder_mismatch_warning = %T, want object", raw)
	}
	if got["type"] != "holder_currentuser_mismatch" {
		t.Errorf("type = %v, want holder_currentuser_mismatch", got["type"])
	}
	if got["holder_open_id"] != "ou_alice" || got["fresh_open_id"] != "ou_bob" {
		t.Errorf("typed open_ids wrong: %v / %v", got["holder_open_id"], got["fresh_open_id"])
	}
	if got["holder_user_name"] != "Alice" || got["fresh_user_name"] != "Bob" {
		t.Errorf("typed user names wrong: %v / %v", got["holder_user_name"], got["fresh_user_name"])
	}

	// Stderr WARN must still be there for humans tailing 2>&1.
	stderrStr := stderr.String()
	for _, sub := range []string{"[lark-cli]", "WARN", "ou_alice", "ou_bob", "active user"} {
		if !strings.Contains(stderrStr, sub) {
			t.Errorf("JSON mode must still emit human WARN to stderr; missing %q\nfull stderr:\n%s", sub, stderrStr)
		}
	}
}

// JSON-mode counter-test: a clean login (no holder mismatch) must NOT
// emit `holder_mismatch_warning`. JSON consumers branch on key presence —
// silent emission of an empty object would falsely trigger downstream
// alerting on every routine login.
func TestAuthLoginRun_CleanLogin_JSONOmitsHolderMismatchWarning(t *testing.T) {
	keyring.MockInit()
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:        "default",
			AppId:       "cli_test",
			CurrentUser: "ou_alice",
			Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	})
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	// Upstream returns the SAME user — no mismatch.
	stubLoginHTTP(t, reg, "ou_alice", "Alice")
	f.Invocation = cmdutil.InvocationContext{Profile: "default"}

	if err := authLoginRun(&LoginOptions{
		Factory: f, Ctx: context.Background(), Scope: "im:message:send", JSON: true,
	}); err != nil {
		t.Fatalf("clean re-login failed: %v", err)
	}

	payload := lastJSONLine(t, stdout.String())
	if _, ok := payload["holder_mismatch_warning"]; ok {
		t.Errorf("clean login leaked holder_mismatch_warning: %v", payload["holder_mismatch_warning"])
	}
}

// lastJSONLine parses the last non-empty newline-delimited JSON object
// from stdout. authLoginRun in JSON mode emits one NDJSON event per
// state transition (device_authorization, then authorization_complete);
// integration tests checking the success payload key on the last event.
func lastJSONLine(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatalf("stdout is empty")
	}
	last := lines[len(lines)-1]
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(last), &payload); err != nil {
		t.Fatalf("Unmarshal last NDJSON line: %v\nline=%s", err, last)
	}
	return payload
}

// Poll-path × JSON-mode cell of the test matrix: the headless --no-wait /
// --device-code workflow run with --json must ALSO surface the structured
// `holder_mismatch_warning` field. The poll path runs through
// authLoginPollDeviceCode, which dispatches to enforceLoginHolderGate —
// the same helper the immediate path uses, but a future refactor that
// drops the JSON wiring at this site (e.g. a poll-only payload helper
// that forgets the holderWarning param) would silently regress only the
// headless workflow. The immediate-path JSON test plus the poll-path
// stderr test cannot together prove this cell; this test does.
func TestAuthLoginPollDeviceCode_LegacyReLoginWorkflow_JSONIncludesHolderMismatchWarning(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	t.Setenv("HOME", t.TempDir())

	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:        "default",
			AppId:       "cli_test",
			CurrentUser: "ou_alice",
			Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	// Same two-stub pattern as the stderr-only poll-path test: only token
	// + user-info; device-authorization is never hit on the poll path and
	// the registry's Verify() check fails on unmatched stubs.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathOAuthTokenV2,
		Body: map[string]interface{}{
			"access_token":             "user-access-token",
			"refresh_token":            "refresh-token",
			"expires_in":               7200,
			"refresh_token_expires_in": 604800,
			"scope":                    "im:message:send offline_access",
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    larkauth.PathUserInfoV1,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"open_id":  "ou_bob",
				"union_id": "uid_ou_bob",
				"name":     "Bob",
			},
		},
	})

	if err := saveLoginRequestedScope("device-code", "im:message:send"); err != nil {
		t.Fatalf("saveLoginRequestedScope: %v", err)
	}

	// Critical for THIS cell: NO --user, NO env (legacy workflow), JSON: true,
	// DeviceCode set (poll path). All four conditions must coincide for
	// the regression we're guarding against to be reachable.
	f.Invocation = cmdutil.InvocationContext{Profile: "default"}
	if err := authLoginRun(&LoginOptions{
		Factory:    f,
		Ctx:        context.Background(),
		DeviceCode: "device-code",
		JSON:       true,
	}); err != nil {
		t.Fatalf("poll-path JSON legacy re-login regressed: %v", err)
	}

	// The poll path emits a SINGLE NDJSON line (no device_authorization
	// event — that already happened in the prior --no-wait invocation).
	// The structured warning rides on that one line. lastJSONLine handles
	// both cases — it just takes the last non-empty line.
	payload := lastJSONLine(t, stdout.String())
	if payload["event"] != "authorization_complete" {
		t.Errorf("event = %v, want authorization_complete", payload["event"])
	}
	raw, ok := payload["holder_mismatch_warning"]
	if !ok {
		t.Fatalf("poll-path JSON payload missing holder_mismatch_warning; payload=%v", payload)
	}
	got, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("holder_mismatch_warning = %T, want object", raw)
	}
	if got["type"] != "holder_currentuser_mismatch" {
		t.Errorf("poll-path type = %v, want holder_currentuser_mismatch", got["type"])
	}
	if got["holder_open_id"] != "ou_alice" || got["fresh_open_id"] != "ou_bob" {
		t.Errorf("poll-path open_ids wrong: %v / %v", got["holder_open_id"], got["fresh_open_id"])
	}
	if got["holder_user_name"] != "Alice" || got["fresh_user_name"] != "Bob" {
		t.Errorf("poll-path user names wrong: %v / %v", got["holder_user_name"], got["fresh_user_name"])
	}

	// Stderr WARN must STILL be emitted on the poll path even in JSON
	// mode — the dual-channel contract is the same. A regression that
	// silenced stderr when stdout was JSON would hide the advisory from
	// humans who tail 2>&1 their headless invocations.
	stderrStr := stderr.String()
	for _, sub := range []string{"[lark-cli]", "WARN", "ou_alice", "ou_bob", "active user"} {
		if !strings.Contains(stderrStr, sub) {
			t.Errorf("poll-path JSON mode must still emit human WARN to stderr; missing %q\nfull stderr:\n%s", sub, stderrStr)
		}
	}

	// Persistence post-state on the poll path's JSON cell mirrors the
	// stderr-only poll test: Bob appended, Alice preserved, CurrentUser
	// unchanged. JSON-vs-stderr is just an output-channel switch; it
	// must not bend the multi-user write contract.
	saved, _ := core.LoadMultiAppConfig()
	users := saved.Apps[0].Users
	if len(users) != 2 || users[0].UserOpenId != "ou_alice" || users[1].UserOpenId != "ou_bob" {
		t.Errorf("Users post-state wrong on poll JSON path: %#v", users)
	}
	if saved.Apps[0].CurrentUser != "ou_alice" {
		t.Errorf("CurrentUser = %q, want ou_alice (poll JSON path must not silently switch)", saved.Apps[0].CurrentUser)
	}
	if got := larkauth.GetStoredToken("cli_test", "ou_bob"); got == nil {
		t.Error("Bob's token slot is empty — poll JSON path login did not actually proceed")
	}
}
