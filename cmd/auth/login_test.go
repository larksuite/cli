// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/command"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/commandhost"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/zalando/go-keyring"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

// builtinResolver resolves domains from the built-in shortcut set. Tests that
// are not specifically about external command sets assert against exactly what
// a distribution built without cmd.WithCommandSets sees.
func builtinResolver() domainResolver {
	return newDomainResolver(shortcuts.AllShortcuts())
}

type businessArgs struct {
	ChatID string `flag:"chat-id" schema:"required;minLength=1" doc:"chat identifier"`
}

type businessData struct {
	ChatID string `json:"chat_id" schema:"required" doc:"chat identifier"`
}

func TestSuggestDomain_PrefixMatch(t *testing.T) {
	known := map[string]bool{
		"calendar": true,
		"task":     true,
		"drive":    true,
		"im":       true,
	}

	// Input is prefix of known domain
	if s := suggestDomain("cal", known); s != "calendar" {
		t.Errorf("expected 'calendar', got %q", s)
	}

	// Known domain is prefix of input
	if s := suggestDomain("calendar_extra", known); s != "calendar" {
		t.Errorf("expected 'calendar', got %q", s)
	}
}

func TestSuggestDomain_NoMatch(t *testing.T) {
	known := map[string]bool{
		"calendar": true,
		"task":     true,
	}

	if s := suggestDomain("zzz", known); s != "" {
		t.Errorf("expected empty suggestion, got %q", s)
	}
}

func TestSuggestDomain_ExactMatch(t *testing.T) {
	known := map[string]bool{
		"calendar": true,
	}

	// Exact match: input is prefix of known AND known is prefix of input
	if s := suggestDomain("calendar", known); s != "calendar" {
		t.Errorf("expected 'calendar', got %q", s)
	}
}

func TestNormalizeScopeInput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"single", "vc:note:read", "vc:note:read"},
		{"comma", "vc:note:read,vc:meeting.meetingevent:read", "vc:note:read vc:meeting.meetingevent:read"},
		{"space", "vc:note:read vc:meeting.meetingevent:read", "vc:note:read vc:meeting.meetingevent:read"},
		{"comma_and_spaces", "vc:note:read, vc:meeting.meetingevent:read", "vc:note:read vc:meeting.meetingevent:read"},
		{"mixed_separators", "a, b\tc\nd  e", "a b c d e"},
		{"trim_and_dedup", "  a , b , a  ", "a b"},
		{"trailing_separators", "a,b,,", "a b"},
		{"only_separators", " , , ", ""},
		{"tab_separated", "im:message:send\toffline_access", "im:message:send offline_access"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeScopeInput(tc.in); got != tc.want {
				t.Errorf("normalizeScopeInput(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestShortcutSupportsIdentity_DefaultUser(t *testing.T) {
	// Empty AuthTypes defaults to ["user"]
	sc := common.Shortcut{AuthTypes: nil}
	if !shortcutSupportsIdentity(sc, "user") {
		t.Error("expected default to support 'user'")
	}
	if shortcutSupportsIdentity(sc, "bot") {
		t.Error("expected default to NOT support 'bot'")
	}
}

func TestShortcutSupportsIdentity_ExplicitTypes(t *testing.T) {
	sc := common.Shortcut{AuthTypes: []string{"user", "bot"}}
	if !shortcutSupportsIdentity(sc, "user") {
		t.Error("expected to support 'user'")
	}
	if !shortcutSupportsIdentity(sc, "bot") {
		t.Error("expected to support 'bot'")
	}
	if shortcutSupportsIdentity(sc, "tenant") {
		t.Error("expected to NOT support 'tenant'")
	}
}

func TestShortcutSupportsIdentity_BotOnly(t *testing.T) {
	sc := common.Shortcut{AuthTypes: []string{"bot"}}
	if shortcutSupportsIdentity(sc, "user") {
		t.Error("expected bot-only to NOT support 'user'")
	}
	if !shortcutSupportsIdentity(sc, "bot") {
		t.Error("expected bot-only to support 'bot'")
	}
}

func TestCompleteDomain(t *testing.T) {
	want := builtinResolver().sorted("")
	if len(want) == 0 {
		t.Skip("no from_meta data available")
	}

	// Complete from empty prefix
	completions := builtinResolver().complete("", "")
	if len(completions) == 0 {
		t.Fatal("expected completions for empty prefix")
	}
	if !reflect.DeepEqual(completions, want) {
		t.Errorf("complete() = %v, want %v", completions, want)
	}
	if !slices.Contains(builtinResolver().complete("not", ""), "note") {
		t.Error("complete() omitted shortcut-only note domain")
	}

	// Complete with partial prefix
	completions = builtinResolver().complete("cal", "")
	for _, c := range completions {
		if c != "calendar" && c[:3] != "cal" {
			t.Errorf("unexpected completion %q for prefix 'cal'", c)
		}
	}
}

func TestCompleteDomain_CommaSeparated(t *testing.T) {
	projects := registry.ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	// After a comma, should complete the next segment
	completions := builtinResolver().complete("calendar,", "")
	for _, c := range completions {
		if c[:9] != "calendar," {
			t.Errorf("expected 'calendar,' prefix, got %q", c)
		}
	}
}

func TestAllKnownDomains(t *testing.T) {
	domains := builtinResolver().allKnown("")
	if len(domains) == 0 {
		t.Fatal("expected non-empty known domains")
	}

	// Should include from_meta projects
	for _, p := range registry.ListFromMetaProjects() {
		if !domains[p] {
			t.Errorf("expected from_meta project %q in known domains", p)
		}
	}
}

func TestSortedKnownDomains(t *testing.T) {
	sorted := builtinResolver().sorted("")
	if len(sorted) == 0 {
		t.Fatal("expected non-empty sorted domains")
	}

	if !sort.StringsAreSorted(sorted) {
		t.Error("expected sorted result")
	}

	// Should match allKnownDomains
	known := builtinResolver().allKnown("")
	if len(sorted) != len(known) {
		t.Errorf("sorted (%d) and known (%d) length mismatch", len(sorted), len(known))
	}
}

func TestShortcutDomainsHaveDescriptions(t *testing.T) {
	seen := make(map[string]struct{})
	for _, shortcut := range shortcuts.AllShortcuts() {
		name := shortcut.Service
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		zhDesc := registry.GetServiceDescription(name, "zh")
		enDesc := registry.GetServiceDescription(name, "en")
		if zhDesc == "" {
			t.Errorf("missing zh description for shortcut-only domain %q", name)
		}
		if enDesc == "" {
			t.Errorf("missing en description for shortcut-only domain %q", name)
		}
	}
}
func TestCollectScopesForDomains(t *testing.T) {
	projects := registry.ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	scopes := builtinResolver().scopesFor([]string{"calendar"}, "user", "")
	if len(scopes) == 0 {
		t.Fatal("expected non-empty scopes for calendar domain")
	}

	// Should be sorted
	if !sort.StringsAreSorted(scopes) {
		t.Error("expected sorted result")
	}

	// Should include at least the API scopes
	apiScopes := registry.CollectScopesForProjects([]string{"calendar"}, "user")
	for _, s := range apiScopes {
		found := false
		for _, cs := range scopes {
			if cs == s {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("API scope %q missing from collectScopesForDomains result", s)
		}
	}
}

func TestCollectScopesForDomains_NonexistentDomain(t *testing.T) {
	scopes := builtinResolver().scopesFor([]string{"nonexistent_domain_xyz"}, "user", "")
	if len(scopes) != 0 {
		t.Errorf("expected empty scopes for nonexistent domain, got %d", len(scopes))
	}
}
func TestExternalShortcutScopesParticipateInAuthDomainResolution(t *testing.T) {
	registered := []common.Shortcut{{
		Service: "im", Command: "+business-auth", AuthTypes: []string{"user"},
		UserScopes: []string{"im:business.scope:read"},
	}}
	domains := newDomainResolver(registered).allKnown("")
	if !domains["im"] {
		t.Fatal("external shortcut domain is missing from auth domains")
	}
	scopes := newDomainResolver(registered).scopesFor([]string{"im"}, "user", "")
	if !slices.Contains(scopes, "im:business.scope:read") {
		t.Fatalf("external shortcut scope is missing: %v", scopes)
	}
}

// A distribution's business command must have its declared scopes reach
// auth login --domain. This walks the chain a real wrapper walks --
// command.Define, commandhost.CompileSets, shortcuts.AllShortcutsWithExternal --
// so dropping the snapshot anywhere along it fails here instead of shipping a
// login that cannot request the distribution's own scopes. The assertion goes
// through the login command's observable behaviour rather than an internal
// field, because the snapshot is deliberately not part of LoginOptions.
func TestCompiledBusinessScopesReachLoginDomainResolution(t *testing.T) {
	const businessScope = "im:business.compiled:read"

	declaration := command.Define(command.Definition[businessArgs, businessData]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+business-compiled", Description: "Business command", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{businessScope}},
			}},
		},
		Hooks: command.Hooks[businessArgs, businessData]{
			Execute: func(_ context.Context, _ command.CommandContext, args *businessArgs) (command.Result[businessData], error) {
				return command.Success(businessData{ChatID: args.ChatID}), nil
			},
		},
	})
	compiled, err := commandhost.CompileSets([]command.Set{{
		Domain:   command.ExtendDomain(command.DomainIm),
		Commands: []command.Command{declaration},
	}})
	if err != nil {
		t.Fatal(err)
	}
	registered, err := shortcuts.AllShortcutsWithExternal(compiled)
	if err != nil {
		t.Fatal(err)
	}

	// Guard against a false positive: the scope must be absent from the
	// built-in set, or this test would pass without the snapshot arriving.
	if builtin := builtinResolver().scopesFor([]string{"im"}, "user", ""); slices.Contains(builtin, businessScope) {
		t.Fatalf("%q is a built-in im scope, so it cannot prove the snapshot arrived", businessScope)
	}
	if scopes := newDomainResolver(registered).scopesFor([]string{"im"}, "user", ""); !slices.Contains(scopes, businessScope) {
		t.Fatalf("compiled business scope %q never reached domain resolution: %v", businessScope, scopes)
	}
}

// One process can build several command trees, and each tree's login must
// resolve against the snapshot it was handed. A distribution's business scopes
// belong to that distribution alone, so a later build without command sets
// cannot inherit them -- the reason the snapshot is a constructor argument
// rather than package-level state.
func TestEachLoginBuildResolvesAgainstItsOwnSnapshot(t *testing.T) {
	const businessScope = "im:business.isolated:read"
	withBusiness := append(shortcuts.AllShortcuts(), common.Shortcut{
		Service: "im", Command: "+business-isolated", AuthTypes: []string{"user"},
		UserScopes: []string{businessScope},
	})

	first := newDomainResolver(withBusiness)
	second := newDomainResolver(shortcuts.AllShortcuts())

	if scopes := first.scopesFor([]string{"im"}, "user", core.BrandFeishu); !slices.Contains(scopes, businessScope) {
		t.Fatalf("first build lost its own business scope %q: %v", businessScope, scopes)
	}
	if scopes := second.scopesFor([]string{"im"}, "user", core.BrandFeishu); slices.Contains(scopes, businessScope) {
		t.Fatalf("second build inherited the first build's business scope %q", businessScope)
	}
}

// The login command must resolve --domain against the snapshot it was
// constructed with. The help text is the observable projection of that snapshot,
// so a business command's domain has to survive into it.
func TestLoginHelpListsDomainsFromTheGivenSnapshot(t *testing.T) {
	registered := append(shortcuts.AllShortcuts(), common.Shortcut{
		Service: "im", Command: "+business-help", AuthTypes: []string{"user"},
		UserScopes: []string{"im:business.help:read"},
	})
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	usage := newCmdAuthLogin(f, nil, registered).Flag("domain").Usage
	for _, want := range newDomainResolver(registered).sorted(core.BrandFeishu) {
		if !strings.Contains(usage, want) {
			t.Fatalf("--domain usage omits %q resolved from the snapshot:\n%s", want, usage)
		}
	}
}

// A scope-less domain stays addressable via --domain (main behavior): it
// passes domain validation and fails later on scope resolution, not with
// "unknown domain".
func TestScopelessDomainStaysAddressableViaDomainFlag(t *testing.T) {
	known := builtinResolver().allKnown("")
	if !known["event"] {
		t.Fatal("event must remain in allKnownDomains to match main behavior")
	}
	if scopes := builtinResolver().scopesFor([]string{"event"}, "user", ""); len(scopes) != 0 {
		t.Fatalf("event scopes = %v, want none", scopes)
	}
}

func TestAuthLoginHelpMatchesKnownDomains(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	factory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	login := NewCmdAuthLogin(factory, nil)
	domainFlag := login.Flags().Lookup("domain")
	if domainFlag == nil {
		t.Fatal("auth login --domain flag is missing")
	}
	names := builtinResolver().sorted("")
	want := "available: " + strings.Join(names, ", ") + ", all"
	if !strings.Contains(domainFlag.Usage, want) {
		t.Fatalf("domain help = %q, want %q", domainFlag.Usage, want)
	}
}

// TestEveryRegisteredDomain_HasBilingualDescription reconciles every registered
// business domain against service_descriptions.json.
//
// TestGetDomainMetadata_HasTitleAndDescription does not cover this. It asserts on
// buildDomainMeta's output, which falls back to the typed service spec when the
// config has no entry — and that spec carries one string for both languages. So a
// domain missing from the config still passes it, while rendering (for example) a
// Chinese description in English `--help`. attendance and mindnotes shipped that
// way. Asserting on the registry getters checks the config itself, before any
// fallback can paper over the gap.
//
// EmbeddedServicesTyped is the overlay-free parse, so this stays deterministic
// whatever ~/.lark-cli/cache/remote_meta.json happens to hold on the machine.
// A domain that only ever arrives via remote overlay is out of reach here.
func TestEveryRegisteredDomain_HasBilingualDescription(t *testing.T) {
	origin := make(map[string]string) // domain name → where it is registered
	for _, svc := range registry.EmbeddedServicesTyped() {
		origin[svc.Name] = "embedded API meta"
	}
	for _, sc := range shortcuts.AllShortcuts() {
		if _, ok := origin[sc.Service]; !ok {
			origin[sc.Service] = "shortcut registration"
		}
	}
	if len(origin) == 0 {
		t.Fatal("no registered domains found — the reconciliation would be vacuous")
	}

	names := make([]string, 0, len(origin))
	for name := range origin {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		for _, lang := range []string{"en", "zh"} {
			if registry.GetServiceTitle(name, lang) == "" {
				t.Errorf("domain %q (registered via %s) has no %s title in service_descriptions.json",
					name, origin[name], lang)
			}
			if registry.GetServiceDescription(name, lang) == "" {
				t.Errorf("domain %q (registered via %s) has no %s description in service_descriptions.json",
					name, origin[name], lang)
			}
		}
	}
}
func TestGenericUserAuthorizationStartCommandPassesLoginValidation(t *testing.T) {
	const startCommand = "lark-cli auth login --recommend --no-wait --json"
	if hint := recovery.UserAuthorization().String(); !strings.Contains(hint, startCommand) {
		t.Fatalf("generic recovery = %q, want executable start command %q", hint, startCommand)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathDeviceAuthorization,
		Body: map[string]interface{}{
			"device_code":               "device-code",
			"user_code":                 "user-code",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=123",
			"expires_in":                240,
			"interval":                  5,
		},
	})

	err := authLoginRun(&LoginOptions{
		Factory:   f,
		Ctx:       context.Background(),
		Recommend: true,
		NoWait:    true,
		JSON:      true,
	}, builtinResolver())
	if err != nil {
		t.Fatalf("generic recovery start command failed before returning a verification URL: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode login response: %v\nstdout=%s", err, stdout.String())
	}
	if got := payload["verification_url"]; got != "https://example.com/verify?code=123" {
		t.Fatalf("verification_url = %#v, want mocked URL", got)
	}
	hint, ok := payload["hint"].(string)
	if !ok {
		t.Fatalf("hint = %#v, want string", payload["hint"])
	}
	if strings.Contains(hint, "lark-cli auth login --no-wait --json") {
		t.Errorf("successful start response recommends an invalid optionless retry: %q", hint)
	}
	for _, want := range []string{"same `--scope`, `--domain`, or `--recommend` selection", "any `--exclude` values", "`--no-wait --json`"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint = %q, want executable fresh-login guidance containing %q", hint, want)
		}
	}
	reg.Verify(t)
}

// TestAuthLoginRun_JSONAbort_StdoutEventOnly_StderrEmpty pins the
// contract that when --json is set and pollDeviceToken returns OK=false,
// stdout carries the structured authorization_failed event and stderr is
// NOT polluted with a typed envelope. The returned error is a bare
// BareError with ExitAuth so the dispatcher only propagates the exit code
// without emitting a second envelope on top of the JSON event.
func TestAuthLoginRun_JSONAbort_StdoutEventOnly_StderrEmpty(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)

	original := pollDeviceToken
	t.Cleanup(func() { pollDeviceToken = original })
	pollDeviceToken = func(ctx context.Context, httpClient *http.Client, appId, appSecret string, brand core.LarkBrand, deviceCode string, interval, expiresIn int, errOut io.Writer) *larkauth.DeviceFlowResult {
		return &larkauth.DeviceFlowResult{OK: false, Message: "user denied"}
	}

	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathDeviceAuthorization,
		Body: map[string]interface{}{
			"device_code":               "device-code",
			"user_code":                 "user-code",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=123",
			"expires_in":                240,
			"interval":                  0,
		},
	})

	err := authLoginRun(&LoginOptions{
		Factory: f,
		Ctx:     context.Background(),
		Scope:   "im:message:send",
		JSON:    true,
	}, builtinResolver())
	if err == nil {
		t.Fatal("expected error for aborted authorization")
	}
	if gotCode := output.ExitCodeOf(err); gotCode != output.ExitAuth {
		t.Fatalf("exit code = %d, want %d", gotCode, output.ExitAuth)
	}

	// stdout: device_authorization event + authorization_failed event,
	// the latter carrying the abort message as a structured field.
	stdoutStr := stdout.String()
	if !strings.Contains(stdoutStr, `"event":"authorization_failed"`) {
		t.Errorf("stdout missing authorization_failed event, got: %s", stdoutStr)
	}
	if !strings.Contains(stdoutStr, "user denied") {
		t.Errorf("stdout missing abort message, got: %s", stdoutStr)
	}

	// stderr must NOT carry a typed envelope: ErrBare propagates the exit
	// code only, so the dispatcher emits nothing on stderr. The waiting-auth
	// log line goes through the JSON-mode no-op `log` helper so it is also
	// suppressed in JSON mode.
	stderrStr := stderr.String()
	if strings.Contains(stderrStr, `"type":"authentication"`) {
		t.Errorf("stderr should not contain typed envelope, got: %s", stderrStr)
	}
	if strings.Contains(stderrStr, `"error"`) {
		t.Errorf("stderr should not contain JSON envelope fields, got: %s", stderrStr)
	}

	// Returned error must be the bare *output.BareError signal (no envelope).
	var bareErr *output.BareError
	if !errors.As(err, &bareErr) {
		t.Fatalf("expected *output.BareError, got %T: %v", err, err)
	}
	if bareErr.Code != output.ExitAuth {
		t.Fatalf("BareError.Code = %d, want %d", bareErr.Code, output.ExitAuth)
	}
}

func TestAuthLoginRun_JSONWriteFailure_NoWaitReturnsWriterError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})
	f.IOStreams.Out = failWriter{}

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathDeviceAuthorization,
		Body: map[string]interface{}{
			"device_code":               "device-code",
			"user_code":                 "user-code",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=123",
			"expires_in":                240,
			"interval":                  5,
		},
	})

	err := authLoginRun(&LoginOptions{
		Factory: f,
		Ctx:     context.Background(),
		Scope:   "im:message:send",
		NoWait:  true,
		JSON:    true,
	}, builtinResolver())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to write JSON output") {
		t.Fatalf("error = %v, want JSON write failure", err)
	}
}

func TestAuthLoginRun_NoWaitJSONHintIncludesRawURLGuidance(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathDeviceAuthorization,
		Body: map[string]interface{}{
			"device_code":               "device-code",
			"user_code":                 "user-code",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=123",
			"expires_in":                240,
			"interval":                  5,
		},
	})

	err := authLoginRun(&LoginOptions{
		Factory: f,
		Ctx:     context.Background(),
		Scope:   "im:message:send",
		NoWait:  true,
	}, builtinResolver())
	if err != nil {
		t.Fatalf("authLoginRun() error = %v", err)
	}

	dec := json.NewDecoder(strings.NewReader(stdout.String()))
	var data map[string]interface{}
	if err := dec.Decode(&data); err != nil {
		t.Fatalf("Decode(stdout first event) error = %v, stdout=%q", err, stdout.String())
	}
	hint, _ := data["hint"].(string)
	for _, want := range []string{
		"MUST generate QR code AND display it",
		"lark-cli auth qrcode",
		"Prefer PNG QR code (--output)",
		"use ASCII (--ascii) only when the user explicitly requests it",
		"This is a required step, do NOT skip it",
		"CRITICAL",
		"You MUST include the QR image in your response",
		"Generating the file alone is NOT enough",
		"image tags, inline images, or file attachments",
		"Display order",
		"place the QR code image below the URL",
		"opaque string",
		"cannot be modified",
		"final message of the turn",
		"return control to the user",
		"do not block on --device-code in the same turn",
		"come back and notify",
		"YOU must execute",
		"lark-cli auth login --device-code <device_code>",
		"Do NOT cache",
		"same `--scope`, `--domain`, or `--recommend` selection",
		"any `--exclude` values",
		"`--no-wait --json`",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint missing %q, got:\n%s", want, hint)
		}
	}
	for _, unwanted := range []string{
		"Then immediately execute",
		"Do not instruct the user to run this command themselves",
		"lark-cli auth login --no-wait --json",
	} {
		if strings.Contains(hint, unwanted) {
			t.Fatalf("hint should not contain %q, got:\n%s", unwanted, hint)
		}
	}
}

func TestNoWaitAgentHint_DefaultBytesStable(t *testing.T) {
	const wantSHA256 = "bd1000350f418a4353807c45c68e1ee073127366bf9d8dd8a0a0f797e0adf8b7"
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(noWaitAgentHint(recovery.RenderContext{})))); got != wantSHA256 {
		t.Fatalf("default no-wait hint digest = %s, want legacy %s", got, wantSHA256)
	}
}

func TestAuthLoginRun_NoWaitJSONHintPreservesExplicitProfile(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "team-beta",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})
	f.Invocation.Profile = "team-beta"

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathDeviceAuthorization,
		Body: map[string]interface{}{
			"device_code":               "device-code",
			"user_code":                 "user-code",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=123",
			"expires_in":                240,
			"interval":                  5,
		},
	})

	if err := authLoginRun(&LoginOptions{
		Factory: f,
		Ctx:     context.Background(),
		Scope:   "im:message:send",
		NoWait:  true,
		JSON:    true,
	}, builtinResolver()); err != nil {
		t.Fatalf("authLoginRun() error = %v", err)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(strings.NewReader(stdout.String())).Decode(&data); err != nil {
		t.Fatalf("Decode(stdout) error = %v, stdout=%q", err, stdout.String())
	}
	hint, _ := data["hint"].(string)
	for _, want := range []string{
		"`lark-cli auth login --profile='team-beta' --device-code <device_code>`",
		"rerun `lark-cli auth login --profile='team-beta'` with the same",
	} {
		if !strings.Contains(hint, want) {
			t.Errorf("profile-aware no-wait JSON hint missing %q: %s", want, hint)
		}
	}
	for _, stale := range []string{
		"`lark-cli auth login --device-code <device_code>`",
		"rerun `lark-cli auth login` with the same",
	} {
		if strings.Contains(hint, stale) {
			t.Errorf("profile-aware no-wait JSON hint retained stale command %q: %s", stale, hint)
		}
	}
}

func TestAuthLoginRun_JSONWriteFailure_DeviceAuthorizationReturnsWriterError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})
	f.IOStreams.Out = failWriter{}

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathDeviceAuthorization,
		Body: map[string]interface{}{
			"device_code":               "device-code",
			"user_code":                 "user-code",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=123",
			"expires_in":                240,
			"interval":                  5,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := authLoginRun(&LoginOptions{
		Factory: f,
		Ctx:     ctx,
		Scope:   "im:message:send",
		JSON:    true,
	}, builtinResolver())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to write JSON output") {
		t.Fatalf("error = %v, want JSON write failure", err)
	}
}

func TestAuthLoginRun_JSONDeviceAuthorizationAgentHintIncludesRawURLGuidance(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathDeviceAuthorization,
		Body: map[string]interface{}{
			"device_code":               "device-code",
			"user_code":                 "user-code",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=123",
			"expires_in":                240,
			"interval":                  5,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := authLoginRun(&LoginOptions{
		Factory: f,
		Ctx:     ctx,
		Scope:   "im:message:send",
		JSON:    true,
	}, builtinResolver())
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}

	dec := json.NewDecoder(strings.NewReader(stdout.String()))
	var data map[string]interface{}
	if err := dec.Decode(&data); err != nil {
		t.Fatalf("Decode(stdout first event) error = %v, stdout=%q", err, stdout.String())
	}
	hint, _ := data["agent_hint"].(string)
	for _, want := range []string{
		"timeout >= 600s",
		"本轮最终消息",
		"结束本轮",
		"用户回复已完成授权",
		"不要在同一轮里展示 URL 后立刻阻塞执行 --device-code",
		"必须生成二维码并展示",
		"lark-cli auth qrcode",
		"优先生成 PNG 二维码（--output）",
		"仅当用户明确要求时才使用 ASCII（--ascii）",
		"生成后必须在回复中展示图片",
		"仅生成文件不算完成",
		"image 标签或内联图片",
		"二维码图片置于 URL 下方完整展示",
		"URL 输出规则",
		"opaque string",
		"不要做任何修改",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("agent_hint missing %q, got:\n%s", want, hint)
		}
	}
}
func TestAllKnownDomains_ExcludesAuthDomainChildren(t *testing.T) {
	domains := builtinResolver().allKnown("")
	if domains["whiteboard"] {
		t.Error("whiteboard should not appear in known auth domains (it has auth_domain=docs)")
	}
	if !domains["docs"] {
		t.Error("docs should still be a known auth domain")
	}
}

func TestCollectScopesForDomains_ExpandsAuthDomainChildren(t *testing.T) {
	scopes := builtinResolver().scopesFor([]string{"docs"}, "user", "")
	// docs domain should include whiteboard shortcut scopes (board:whiteboard:*)
	found := false
	for _, s := range scopes {
		if strings.HasPrefix(s, "board:whiteboard:") {
			found = true
			break
		}
	}
	if !found {
		t.Error("builtinResolver().scopesFor([docs]) should include whiteboard scopes (board:whiteboard:*)")
	}
}
func TestFilterBatchExcludedScopes(t *testing.T) {
	got := filterBatchExcludedScopes([]string{"im:message", "im:message.send_as_user", "im:message:readonly"})
	want := []string{"im:message", "im:message:readonly"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	clean := []string{"im:message", "calendar:calendar:read"}
	if got := filterBatchExcludedScopes(clean); !slices.Equal(got, clean) {
		t.Errorf("clean input changed: got %v, want %v", got, clean)
	}
	if got := filterBatchExcludedScopes(nil); len(got) != 0 {
		t.Errorf("nil input: got %v, want empty", got)
	}
}

func TestBatchExcludedScopes_ContainsSendAsUser(t *testing.T) {
	if !batchExcludedScopes["im:message.send_as_user"] {
		t.Fatal("batchExcludedScopes must contain im:message.send_as_user")
	}
}

func TestFilterBatchExcludedScopes_OnImDomainSet(t *testing.T) {
	raw := builtinResolver().scopesFor([]string{"im"}, "user", "")
	if !slices.Contains(raw, "im:message.send_as_user") {
		t.Fatal("precondition: im domain set must contain im:message.send_as_user (on-demand grant source intact)")
	}
	filtered := filterBatchExcludedScopes(raw)
	if slices.Contains(filtered, "im:message.send_as_user") {
		t.Error("filtered set must not contain im:message.send_as_user")
	}
	if !slices.Contains(filtered, "im:message") {
		t.Error("filtered set should still contain im:message")
	}
}

func TestAuthLoginRun_BatchExcludesSendAsUser(t *testing.T) {
	// Isolate the requested-scope cache: --no-wait persists scopes under the
	// config dir, so pin it to a temp dir to avoid touching the real one.
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	extractScope := func(body []byte) string {
		v, _ := url.ParseQuery(string(body))
		return v.Get("scope")
	}
	stubBody := map[string]interface{}{
		"device_code": "dc", "user_code": "uc",
		"verification_uri": "https://example.com/v", "verification_uri_complete": "https://example.com/v?c=1",
		"expires_in": 240, "interval": 5,
	}

	// (a) --domain im must NOT request send_as_user in the batch set
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	stub := &httpmock.Stub{Method: "POST", URL: larkauth.PathDeviceAuthorization, Body: stubBody}
	reg.Register(stub)
	if err := authLoginRun(&LoginOptions{
		Factory: f, Ctx: context.Background(),
		Domains: []string{"im"}, NoWait: true, JSON: true,
	}, builtinResolver()); err != nil {
		t.Fatalf("authLoginRun --domain im: %v", err)
	}
	scope := extractScope(stub.CapturedBody)
	if strings.Contains(scope, "im:message.send_as_user") {
		t.Errorf("--domain im requested send_as_user; scope=%q", scope)
	}
	if !strings.Contains(scope, "im:message") {
		t.Errorf("--domain im missing im:message; scope=%q", scope)
	}

	// (b) explicit --scope re-adds it
	f2, _, _, reg2 := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	stub2 := &httpmock.Stub{Method: "POST", URL: larkauth.PathDeviceAuthorization, Body: stubBody}
	reg2.Register(stub2)
	if err := authLoginRun(&LoginOptions{
		Factory: f2, Ctx: context.Background(),
		Domains: []string{"im"}, Scope: "im:message.send_as_user", NoWait: true, JSON: true,
	}, builtinResolver()); err != nil {
		t.Fatalf("authLoginRun --domain im --scope send_as_user: %v", err)
	}
	if scope2 := extractScope(stub2.CapturedBody); !strings.Contains(scope2, "im:message.send_as_user") {
		t.Errorf("explicit --scope did not re-add send_as_user; scope=%q", scope2)
	}
}

func TestAuthLoginRun_DomainExcludeSendAsUser(t *testing.T) {
	// Regression (P1): `--domain im --exclude im:message.send_as_user` must keep
	// succeeding as it did before the batch-exclusion filter existed. The batch
	// filter drops send_as_user from the effective (wire) set, but --exclude is
	// validated against the pre-filter selected universe, so naming it stays a
	// valid no-op instead of an invalid_argument error. Automations relied on
	// this exact form to dodge the send-as-user approval; do not break them.
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	stubBody := map[string]interface{}{
		"device_code": "dc", "user_code": "uc",
		"verification_uri": "https://example.com/v", "verification_uri_complete": "https://example.com/v?c=1",
		"expires_in": 240, "interval": 5,
	}
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	stub := &httpmock.Stub{Method: "POST", URL: larkauth.PathDeviceAuthorization, Body: stubBody}
	reg.Register(stub)
	if err := authLoginRun(&LoginOptions{
		Factory: f, Ctx: context.Background(),
		Domains: []string{"im"}, Exclude: []string{"im:message.send_as_user"},
		NoWait: true, JSON: true,
	}, builtinResolver()); err != nil {
		t.Fatalf("authLoginRun --domain im --exclude send_as_user: %v", err)
	}
	if stub.CapturedBody == nil {
		t.Fatal("no device authorization request was sent (command errored before the wire call)")
	}
	v, _ := url.ParseQuery(string(stub.CapturedBody))
	scope := v.Get("scope")
	scopeSet := make(map[string]bool)
	for _, s := range strings.Fields(scope) {
		scopeSet[s] = true
	}
	if scopeSet["im:message.send_as_user"] {
		t.Errorf("excluded scope leaked into wire request; scope=%q", scope)
	}
	if !scopeSet["im:message"] {
		t.Errorf("--domain im missing exact im:message; scope=%q", scope)
	}
}

func TestApplyExcludeScopes_ValidatesAgainstUniverse(t *testing.T) {
	// Universe is a superset of the effective (requested) set: it carries a
	// batch-withheld scope that is not on the wire.
	universe := map[string]bool{"im:message": true, "im:message.send_as_user": true}

	// (a) excluding a universe member absent from requested is a valid no-op.
	got, unknown := applyExcludeScopes("im:message", []string{"im:message.send_as_user"}, universe)
	if len(unknown) != 0 {
		t.Fatalf("unexpected unknown: %v", unknown)
	}
	if got != "im:message" {
		t.Errorf("requested changed: got %q, want %q", got, "im:message")
	}

	// (b) a scope outside the universe is still unknown (no typo widening).
	if _, unknown := applyExcludeScopes("im:message", []string{"im:typo"}, universe); len(unknown) != 1 || unknown[0] != "im:typo" {
		t.Errorf("expected unknown [im:typo], got %v", unknown)
	}

	// (c) nil universe validates against requested (pure --scope path unchanged).
	if _, unknown := applyExcludeScopes("im:message", []string{"im:message.send_as_user"}, nil); len(unknown) != 1 || unknown[0] != "im:message.send_as_user" {
		t.Errorf("nil universe should validate against requested; got unknown %v", unknown)
	}
	if kept, unknown := applyExcludeScopes("im:message calendar:calendar:read", []string{"im:message"}, nil); len(unknown) != 0 || kept != "calendar:calendar:read" {
		t.Errorf("nil universe removal: kept=%q unknown=%v", kept, unknown)
	}
}
