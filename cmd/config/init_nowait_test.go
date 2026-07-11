// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// roundTripFunc adapts a function to an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestNoWait_InitiateThenResume_EndToEnd drives the full two-step flow against a
// real local HTTP server: initiate writes the on-disk cache, then a SEPARATE
// resume call polls the same server, succeeds, and persists the new app. Only
// the device_code + the cache bridge the two invocations — exactly as the two
// CLI commands would. (A black-box binary E2E of the success path is impossible
// without a human: endpoints are hardcoded HTTPS and the real device flow needs
// a browser scan, so this in-process run through httptest is the highest-fidelity
// autonomous end-to-end.)
func TestNoWait_InitiateThenResume_EndToEnd(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.FormValue("action") {
		case "begin":
			_, _ = w.Write([]byte(`{"device_code":"E2E-DEVICE-CODE","user_code":"E2E-UC","verification_uri":"https://example.test/verify","expires_in":600,"interval":1}`))
		case "poll":
			_, _ = w.Write([]byte(`{"client_id":"cli_e2e","client_secret":"sec_e2e","user_info":{"tenant_brand":"feishu","open_id":"ou_e2e"}}`))
		default:
			http.Error(w, "unexpected action "+r.FormValue("action"), http.StatusBadRequest)
		}
	}))
	defer ts.Close()
	tsURL, _ := url.Parse(ts.URL)

	// Redirect the registration client to the local test server.
	orig := newRegistrationHTTPClient
	newRegistrationHTTPClient = func() *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			r.URL.Scheme, r.URL.Host = tsURL.Scheme, tsURL.Host
			return http.DefaultTransport.RoundTrip(r)
		})}
	}
	t.Cleanup(func() { newRegistrationHTTPClient = orig })

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	// Step 1 — initiate: should print device_code and write the resume cache.
	initOpts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), Brand: "feishu", New: true, NoWait: true}
	if err := initiateNoWaitAppRegistration(initOpts, nil); err != nil {
		t.Fatalf("initiate: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("initiate stdout not JSON: %v; raw=%s", err, stdout.String())
	}
	if out["device_code"] != "E2E-DEVICE-CODE" {
		t.Fatalf("device_code = %v, want E2E-DEVICE-CODE", out["device_code"])
	}
	if rec, _ := loadInitNoWaitRecord("E2E-DEVICE-CODE"); rec == nil {
		t.Fatal("initiate did not write the resume cache")
	}

	// Step 2 — resume (separate invocation; bridged only by device_code + cache).
	stdout.Reset()
	resumeOpts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), DeviceCode: "E2E-DEVICE-CODE"}
	if err := resumeAppRegistration(resumeOpts); err != nil {
		t.Fatalf("resume: %v", err)
	}

	// The new app must be persisted to config...
	cfg, err := core.LoadMultiAppConfig()
	if err != nil || cfg == nil {
		t.Fatalf("config not persisted: %v", err)
	}
	if app := cfg.CurrentAppConfig(""); app == nil || app.AppId != "cli_e2e" {
		t.Fatalf("persisted app = %+v, want AppId cli_e2e", app)
	}
	// ...the cache cleared after the successful save...
	if rec, _ := loadInitNoWaitRecord("E2E-DEVICE-CODE"); rec != nil {
		t.Error("resume should clear the cache after a successful save")
	}
	// ...and the success JSON emitted.
	if !strings.Contains(stdout.String(), "cli_e2e") {
		t.Errorf("resume stdout missing appId: %s", stdout.String())
	}
}

// stubRT returns a single canned HTTP response for every request.
type stubRT struct {
	status int
	body   string
}

func (s stubRT) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: s.status, Body: io.NopCloser(strings.NewReader(s.body)), Header: make(http.Header)}, nil
}

// seqRT returns successive canned responses (last one repeats), for flows that
// poll more than once (e.g. the Lark-tenant re-poll).
type seqRT struct {
	bodies []string
	i      int
}

func (s *seqRT) RoundTrip(*http.Request) (*http.Response, error) {
	idx := s.i
	if idx >= len(s.bodies) {
		idx = len(s.bodies) - 1
	}
	s.i++
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(s.bodies[idx])), Header: make(http.Header)}, nil
}

// withStubRegistrationClient swaps the registration HTTP client for the test.
func withStubRegistrationClient(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	orig := newRegistrationHTTPClient
	newRegistrationHTTPClient = func() *http.Client { return &http.Client{Transport: rt} }
	t.Cleanup(func() { newRegistrationHTTPClient = orig })
}

// --- cache round-trip ---

func TestInitNoWaitCache_RoundTrip(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	rec := initNoWaitRecord{
		Version:      initNoWaitCacheVersion,
		Brand:        "feishu",
		ProfileName:  "work",
		Lang:         "zh_cn",
		LangExplicit: true,
		Interval:     5,
		ExpiresAt:    time.Now().Unix() + 300,
		ConfigDigest: "abc123",
	}
	const dc = "device-code-xyz"

	if err := saveInitNoWaitRecord(dc, rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := loadInitNoWaitRecord(dc)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("load returned nil for a saved record")
	}
	if *got != rec {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", *got, rec)
	}

	if err := removeInitNoWaitRecord(dc); err != nil {
		t.Fatalf("remove: %v", err)
	}
	got2, err := loadInitNoWaitRecord(dc)
	if err != nil {
		t.Fatalf("load after remove: %v", err)
	}
	if got2 != nil {
		t.Errorf("expected nil after remove, got %+v", got2)
	}
	// Removing a non-existent record must be a no-op, not an error.
	if err := removeInitNoWaitRecord(dc); err != nil {
		t.Errorf("remove of missing record should be nil, got %v", err)
	}
}

func TestInitNoWaitCache_LoadMissing(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	got, err := loadInitNoWaitRecord("never-saved")
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing record, got %+v", got)
	}
}

func TestInitNoWaitCache_VersionMismatchIgnored(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	const dc = "stale-version"
	rec := initNoWaitRecord{Version: initNoWaitCacheVersion + 1, ExpiresAt: time.Now().Unix() + 300}
	if err := saveInitNoWaitRecord(dc, rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := loadInitNoWaitRecord(dc)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for version mismatch, got %+v", got)
	}
	// The stale entry should have been discarded by the load.
	got2, _ := loadInitNoWaitRecord(dc)
	if got2 != nil {
		t.Errorf("stale-version entry was not removed on load")
	}
}

func TestInitNoWaitCacheKey(t *testing.T) {
	// Distinct device codes that a char-replacement sanitizer would collide
	// ("a/b" and "a:b" -> "a_b") must map to distinct keys.
	if initNoWaitCacheKey("a/b") == initNoWaitCacheKey("a:b") {
		t.Error("distinct device codes must not collide on the cache key")
	}
	// Deterministic.
	if initNoWaitCacheKey("xyz") != initNoWaitCacheKey("xyz") {
		t.Error("cache key must be deterministic")
	}
	// sha256 hex: 64 chars, filesystem-safe regardless of input.
	k := initNoWaitCacheKey("has /, :, ;, spaces and 'quotes'")
	if len(k) != 64 {
		t.Errorf("expected 64-char sha256 hex key, got %d: %q", len(k), k)
	}
}

// --- config digest ---

func TestComputeConfigDigest(t *testing.T) {
	if d := computeConfigDigest(nil); d != "" {
		t.Errorf("nil digest = %q, want empty", d)
	}
	cfg1 := &core.MultiAppConfig{Apps: []core.AppConfig{{AppId: "cli_a", Brand: core.BrandFeishu}}}
	cfg1Dup := &core.MultiAppConfig{Apps: []core.AppConfig{{AppId: "cli_a", Brand: core.BrandFeishu}}}
	cfg2 := &core.MultiAppConfig{Apps: []core.AppConfig{{AppId: "cli_b", Brand: core.BrandFeishu}}}

	if computeConfigDigest(cfg1) == "" {
		t.Error("non-nil config digest should be non-empty")
	}
	if computeConfigDigest(cfg1) != computeConfigDigest(cfg1Dup) {
		t.Error("equal configs should produce equal digests")
	}
	if computeConfigDigest(cfg1) == computeConfigDigest(cfg2) {
		t.Error("different configs should produce different digests")
	}
}

// --- failure classification for cache cleanup ---

func TestAppRegShouldClearCache(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"success", nil, true},
		{"denied", larkauth.ErrAppRegDenied, true},
		{"expired", larkauth.ErrAppRegExpired, true},
		{"expired wrapped", fmt.Errorf("%w, please try again", larkauth.ErrAppRegExpired), true},
		{"timeout", larkauth.ErrAppRegTimeout, true},
		{"timeout wrapped", fmt.Errorf("%w, please try again", larkauth.ErrAppRegTimeout), true},
		{"cancelled", larkauth.ErrAppRegCancelled, false},
		{"transient generic", fmt.Errorf("network boom"), false},
		{"missing fields", fmt.Errorf("app registration succeeded but missing client_id or client_secret"), false},
	}
	for _, c := range cases {
		if got := appRegShouldClearCache(c.err); got != c.want {
			t.Errorf("%s: appRegShouldClearCache = %v, want %v", c.name, got, c.want)
		}
	}
}

// --- initiate (stubbed registration client) ---

func TestInitiateNoWaitAppRegistration_WritesCacheAndJSON(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	withStubRegistrationClient(t, stubRT{200, `{"device_code":"dc-abc","user_code":"U-1","verification_uri":"https://open.feishu.cn","expires_in":3600,"interval":5}`})

	opts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), Brand: "feishu", New: true, NoWait: true, ForceInit: true}
	if err := initiateNoWaitAppRegistration(opts, nil); err != nil {
		t.Fatalf("initiate: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout not JSON: %v; raw=%s", err, stdout.String())
	}
	if out["device_code"] != "dc-abc" {
		t.Errorf("device_code = %v, want dc-abc", out["device_code"])
	}
	args, ok := out["resume_args"].([]interface{})
	if !ok || len(args) == 0 || args[len(args)-1] != "--force-init" {
		t.Errorf("resume_args should end with --force-init, got %v", out["resume_args"])
	}

	rec, _ := loadInitNoWaitRecord("dc-abc")
	if rec == nil {
		t.Fatal("cache record not written")
	}
	if rec.Brand != "feishu" || rec.Version != initNoWaitCacheVersion {
		t.Errorf("cache record = %+v", *rec)
	}
}

// --- pollAppRegistrationResume (stubbed client) ---

func TestPollAppRegistrationResume_Success(t *testing.T) {
	c := &http.Client{Transport: stubRT{200, `{"client_id":"cli_x","client_secret":"sec","user_info":{"tenant_brand":"feishu"}}`}}
	res, err := pollAppRegistrationResume(context.Background(), c, "dc", 0, 60, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ClientID != "cli_x" || res.ClientSecret != "sec" {
		t.Errorf("got %+v", res)
	}
}

func TestPollAppRegistrationResume_MissingSecret(t *testing.T) {
	c := &http.Client{Transport: stubRT{200, `{"client_id":"cli_x"}`}}
	if _, err := pollAppRegistrationResume(context.Background(), c, "dc", 0, 60, io.Discard); err == nil {
		t.Error("expected error when client_secret is missing")
	}
}

func TestPollAppRegistrationResume_LarkRetry(t *testing.T) {
	// First poll (feishu endpoint): lark tenant, no secret -> triggers re-poll
	// against the lark endpoint, which returns the secret.
	rt := &seqRT{bodies: []string{
		`{"client_id":"cli_x","client_secret":"","user_info":{"tenant_brand":"lark"}}`,
		`{"client_id":"cli_x","client_secret":"larksec","user_info":{"tenant_brand":"lark"}}`,
	}}
	res, err := pollAppRegistrationResume(context.Background(), &http.Client{Transport: rt}, "dc", 0, 60, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ClientSecret != "larksec" {
		t.Errorf("expected lark re-poll to yield the secret, got %+v", res)
	}
}

// Full resume happy path: stubbed poll succeeds, the app is persisted, and the
// cache is cleared. (runProbe hits the factory's mock client, which has no stub
// and returns an untyped error that runProbe swallows.)
func TestResumeAppRegistration_Success(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	withStubRegistrationClient(t, stubRT{200, `{"client_id":"cli_new","client_secret":"sec","user_info":{"tenant_brand":"feishu"}}`})

	const dc = "resume-ok"
	rec := initNoWaitRecord{
		Version:      initNoWaitCacheVersion,
		Brand:        "feishu",
		Interval:     1, // keep the single poll fast
		ExpiresAt:    time.Now().Unix() + 300,
		ConfigDigest: computeConfigDigest(nil),
	}
	if err := saveInitNoWaitRecord(dc, rec); err != nil {
		t.Fatalf("save: %v", err)
	}

	opts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), DeviceCode: dc}
	if err := resumeAppRegistration(opts); err != nil {
		t.Fatalf("resume: %v", err)
	}

	cfg, _ := core.LoadMultiAppConfig()
	if cfg == nil || cfg.CurrentAppConfig("") == nil || cfg.CurrentAppConfig("").AppId != "cli_new" {
		t.Errorf("config not persisted with new app id: %+v", cfg)
	}
	if got, _ := loadInitNoWaitRecord(dc); got != nil {
		t.Error("cache should be cleared after a successful save")
	}
	if !strings.Contains(stdout.String(), "cli_new") {
		t.Errorf("stdout missing new appId: %s", stdout.String())
	}
}

// A profile-name conflict on the resume save path must surface as the typed
// ValidationError(--name), not be downgraded to an internal/storage error.
func TestResumeAppRegistration_ProfileNameConflict_PreservesValidationError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	withStubRegistrationClient(t, stubRT{200, `{"client_id":"cli_new","client_secret":"sec","user_info":{"tenant_brand":"feishu"}}`})

	// Seed a config whose app id collides with the profile name we resume into.
	seeded := &core.MultiAppConfig{Apps: []core.AppConfig{
		{AppId: "cli_existing", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu},
	}}
	if err := core.SaveMultiAppConfig(seeded); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	loaded, _ := core.LoadMultiAppConfig() // digest must match what resume recomputes

	const dc = "conflict-dc"
	rec := initNoWaitRecord{
		Version:      initNoWaitCacheVersion,
		Brand:        "feishu",
		ProfileName:  "cli_existing", // collides with the existing appId in saveAsProfile
		Interval:     1,
		ExpiresAt:    time.Now().Unix() + 300,
		ConfigDigest: computeConfigDigest(loaded),
	}
	if err := saveInitNoWaitRecord(dc, rec); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	opts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), DeviceCode: dc}
	assertValidationParam(t, resumeAppRegistration(opts), "--name")
}

// --- flag validation (returns before any network) ---

func TestConfigInitRun_NoWaitAndDeviceCodeMutuallyExclusive(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	opts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), NoWait: true, DeviceCode: "x"}
	assertValidationParam(t, configInitRun(opts), "--device-code")
}

func TestConfigInitRun_NoWaitWithAppIDRejected(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	opts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), NoWait: true, AppID: "cli_x"}
	assertValidationParam(t, configInitRun(opts), "--no-wait")
}

// The conflict error must point at the flag the caller actually passed: with
// --device-code (not --no-wait) + --app-id, remediation should name --device-code.
func TestConfigInitRun_DeviceCodeWithAppIDReportsDeviceCode(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	opts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), DeviceCode: "dc", AppID: "cli_x"}
	assertValidationParam(t, configInitRun(opts), "--device-code")
}

// --- resume guards (return before any network) ---

func TestResumeAppRegistration_NoCacheEntry(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	opts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), DeviceCode: "missing-dc"}
	assertValidationParam(t, resumeAppRegistration(opts), "--device-code")
}

func TestResumeAppRegistration_ExpiredClearsCache(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	const dc = "expired-dc"
	rec := initNoWaitRecord{
		Version:   initNoWaitCacheVersion,
		Brand:     "feishu",
		Interval:  5,
		ExpiresAt: time.Now().Unix() - 10, // already past
	}
	if err := saveInitNoWaitRecord(dc, rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	opts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), DeviceCode: dc}
	assertValidationParam(t, resumeAppRegistration(opts), "--device-code")

	if got, _ := loadInitNoWaitRecord(dc); got != nil {
		t.Error("expired cache entry should have been removed")
	}
}

// A cache file that exists but cannot be parsed is a storage failure, not a
// "no pending creation" validation error — the user should fix storage rather
// than assume the device code is bad.
func TestResumeAppRegistration_CorruptCacheIsStorageError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	const dc = "corrupt-dc"
	if err := os.MkdirAll(initNoWaitCacheDir(), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(initNoWaitCachePath(dc), []byte("{ not valid json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	opts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), DeviceCode: dc}
	err := resumeAppRegistration(opts)
	var intErr *errs.InternalError
	if !errors.As(err, &intErr) {
		t.Fatalf("expected *errs.InternalError for unreadable cache, got %T: %v", err, err)
	}
	if p, ok := errs.ProblemOf(err); !ok || p.Subtype != errs.SubtypeStorage {
		t.Fatalf("expected subtype=%q, got problem=%+v", errs.SubtypeStorage, p)
	}
	if errors.Unwrap(err) == nil {
		t.Fatal("expected the underlying cache-read failure to be preserved as a cause")
	}
}

func TestResumeAppRegistration_ConfigDrift(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	const dc = "drift-dc"
	rec := initNoWaitRecord{
		Version:      initNoWaitCacheVersion,
		Brand:        "feishu",
		Interval:     5,
		ExpiresAt:    time.Now().Unix() + 300,
		ConfigDigest: "stale-digest-that-will-not-match-current-config",
	}
	if err := saveInitNoWaitRecord(dc, rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	opts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), DeviceCode: dc}
	assertValidationParam(t, resumeAppRegistration(opts), "--device-code")
}
