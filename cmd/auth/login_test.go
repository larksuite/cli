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

func TestGetDomainMetadataIncludesNote(t *testing.T) {
	for _, domain := range builtinResolver().metadata("zh", "") {
		if domain.Name == "note" {
			return
		}
	}
	t.Fatal("domain metadata must include note so auth login can select vc:note:read")
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

func TestGetDomainMetadata_IncludesFromMeta(t *testing.T) {
	domains := builtinResolver().metadata("zh", "")
	nameSet := make(map[string]bool)
	for _, dm := range domains {
		nameSet[dm.Name] = true
	}

	// from_meta projects must be present
	for _, p := range registry.ListFromMetaProjects() {
		if !nameSet[p] {
			t.Errorf("from_meta project %q missing from getDomainMetadata", p)
		}
	}
}

func TestGetDomainMetadataIncludesAuthorizableShortcutDomains(t *testing.T) {
	domains := builtinResolver().metadata("zh", "")
	nameSet := make(map[string]bool)
	for _, dm := range domains {
		nameSet[dm.Name] = true
	}

	for _, shortcut := range shortcuts.AllShortcuts() {
		if registry.HasAuthDomain(shortcut.Service) || !shortcutHasDeclaredScopes(shortcut) {
			continue
		}
		if !nameSet[shortcut.Service] {
			t.Errorf("authorizable shortcut domain %q missing from getDomainMetadata", shortcut.Service)
		}
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

// The interactive selector shows exactly allKnownDomains minus scope-less
// shortcut-only domains (e.g. event). --domain and the help list keep
// accepting those, matching main: selecting a scope-less domain fails later
// with "no matching scopes found" instead of "unknown domain".
func TestGetDomainMetadataMatchesAllKnownDomainsMinusScopeless(t *testing.T) {
	metadata := builtinResolver().metadata("zh", "")
	known := builtinResolver().allKnown("")
	scopeless := builtinResolver().scopeless()
	if len(scopeless) == 0 {
		t.Fatal("expected at least one scope-less domain (event) to exercise the filter")
	}
	if len(metadata) != len(known)-len(scopeless) {
		t.Fatalf("domain metadata count = %d, want allKnownDomains (%d) minus scopeless (%d)",
			len(metadata), len(known), len(scopeless))
	}
	for _, domain := range metadata {
		if !known[domain.Name] {
			t.Errorf("domain metadata contains %q outside allKnownDomains", domain.Name)
		}
		if scopeless[domain.Name] {
			t.Errorf("interactive selector lists scope-less domain %q", domain.Name)
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

func TestGetDomainMetadata_Sorted(t *testing.T) {
	domains := builtinResolver().metadata("zh", "")
	for i := 1; i < len(domains); i++ {
		if domains[i].Name < domains[i-1].Name {
			t.Errorf("not sorted: %q before %q", domains[i-1].Name, domains[i].Name)
		}
	}
}

func TestGetDomainMetadata_HasTitleAndDescription(t *testing.T) {
	domains := builtinResolver().metadata("zh", "")
	for _, dm := range domains {
		if dm.Title == "" {
			t.Errorf("domain %q has empty Title", dm.Name)
		}
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

func TestAuthLoginRun_NonTerminal_NoFlags_RejectsWithHint(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	// TestFactory has IsTerminal=false by default
	opts := &LoginOptions{Factory: f, Ctx: context.Background()}
	err := authLoginRun(opts, builtinResolver())
	if err == nil {
		t.Fatal("expected error for non-terminal without flags")
	}
	// Should mention specifying scopes
	msg := err.Error()
	if !strings.Contains(msg, "scopes") {
		t.Errorf("expected error to mention scopes, got: %s", msg)
	}
	// Stderr should explain the split-flow path for non-streaming agents.
	stderrStr := stderr.String()
	for _, want := range []string{"--no-wait --json", "final message of the turn", "--device-code"} {
		if !strings.Contains(stderrStr, want) {
			t.Errorf("expected stderr to mention %q, got: %s", want, stderrStr)
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

func TestEnsureRequestedScopesGranted(t *testing.T) {
	issue := ensureRequestedScopesGranted("im:message:send im:message:reply", "im:message:reply", getLoginMsg("en"), nil)
	if issue == nil {
		t.Fatal("expected missing scope issue")
	}
	if !strings.Contains(issue.Message, "im:message:send") {
		t.Fatalf("message %q missing requested scope", issue.Message)
	}
	for _, want := range []string{"Do not retry continuously", "scope being disabled", "lark-cli auth status"} {
		if !strings.Contains(issue.Hint, want) {
			t.Fatalf("hint %q missing %q", issue.Hint, want)
		}
	}
	if got := strings.Join(issue.Summary.Missing, " "); got != "im:message:send" {
		t.Fatalf("Missing = %q", got)
	}
}

func TestBuildLoginScopeSummary(t *testing.T) {
	summary := buildLoginScopeSummary("im:message:send im:message:reply im:message:send", "im:message:reply", "im:message:send im:message:reply im:chat:read")
	if got := strings.Join(summary.Requested, " "); got != "im:message:send im:message:reply" {
		t.Fatalf("Requested = %q", got)
	}
	if got := strings.Join(summary.NewlyGranted, " "); got != "im:message:send" {
		t.Fatalf("NewlyGranted = %q", got)
	}
	if got := strings.Join(summary.AlreadyGranted, " "); got != "im:message:reply" {
		t.Fatalf("AlreadyGranted = %q", got)
	}
	if len(summary.Missing) != 0 {
		t.Fatalf("Missing = %v, want empty", summary.Missing)
	}
	if got := strings.Join(summary.Granted, " "); got != "im:message:send im:message:reply im:chat:read" {
		t.Fatalf("Granted = %q", got)
	}
}

func TestWriteLoginSuccess_JSONIncludesScopeDiff(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	writeLoginSuccess(&LoginOptions{JSON: true}, getLoginMsg("en"), f, "ou_user", "tester", &loginScopeSummary{
		Requested:      []string{"im:message:send", "im:message:reply"},
		NewlyGranted:   []string{"im:message:send"},
		AlreadyGranted: []string{"im:message:reply"},
		Granted:        []string{"im:message:send", "im:message:reply"},
	})

	var data map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	if data["event"] != "authorization_complete" {
		t.Fatalf("event = %v", data["event"])
	}
	if data["scope"] != "im:message:send im:message:reply" {
		t.Fatalf("scope = %v", data["scope"])
	}
	if len(data["newly_granted"].([]interface{})) != 1 {
		t.Fatalf("newly_granted = %#v", data["newly_granted"])
	}
	if len(data["already_granted"].([]interface{})) != 1 {
		t.Fatalf("already_granted = %#v", data["already_granted"])
	}
}

func TestHandleLoginScopeIssue_NonJSONAlignsWithLoginSuccess(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	err := handleLoginScopeIssue(&LoginOptions{}, getLoginMsg("zh"), f, &loginScopeIssue{
		Message: "授权结果异常: 以下请求 scopes 未被授予: im:message:send",
		Hint:    "以上结果是本次授权请求用户最终确认后的结果，请勿持续重试；Scopes 未授予的原因是多样的，如 scope 被禁用；具体原因已通过授权页提示用户。可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
		Summary: &loginScopeSummary{
			Requested: []string{"im:message:send"},
			Missing:   []string{"im:message:send"},
			Granted:   []string{"base:app:copy"},
		},
	}, "ou_user", "tester")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if gotCode := output.ExitCodeOf(err); gotCode != output.ExitAuth {
		t.Fatalf("exit code = %d, want %d", gotCode, output.ExitAuth)
	}
	got := stderr.String()
	for _, want := range []string{
		"授权结果异常: 以下请求 scopes 未被授予: im:message:send",
		"当前授权账号: tester (ou_user)",
		"本次请求 scopes: im:message:send",
		"本次新授予 scopes: （空）",
		"以上结果是本次授权请求用户最终确认后的结果，请勿持续重试",
		"scope 被禁用",
		"lark-cli auth status",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr missing %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "最终已授权 scopes:") {
		t.Fatalf("stderr should not contain final granted scopes, got:\n%s", got)
	}
	if strings.Contains(got, "授权成功") {
		t.Fatalf("stderr should not contain success wording, got:\n%s", got)
	}
	if strings.Contains(got, "本次未授予 scopes:") {
		t.Fatalf("stderr should not duplicate missing scopes, got:\n%s", got)
	}
}

func TestHandleLoginScopeIssue_JSONAlignsWithLoginSuccess(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	err := handleLoginScopeIssue(&LoginOptions{JSON: true}, getLoginMsg("en"), f, &loginScopeIssue{
		Message: "authorization result is abnormal: these requested scopes were not granted: im:message:send",
		Hint:    "Granted scopes: base:app:copy. Check app scopes.",
		Summary: &loginScopeSummary{
			Requested: []string{"im:message:send"},
			Missing:   []string{"im:message:send"},
			Granted:   []string{"base:app:copy"},
		},
	}, "ou_user", "tester")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if gotCode := output.ExitCodeOf(err); gotCode != output.ExitAuth {
		t.Fatalf("exit code = %d, want %d", gotCode, output.ExitAuth)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	if data["event"] != "authorization_complete" {
		t.Fatalf("event = %v", data["event"])
	}
	if data["user_open_id"] != "ou_user" {
		t.Fatalf("user_open_id = %v", data["user_open_id"])
	}
	warning, ok := data["warning"].(map[string]interface{})
	if !ok {
		t.Fatalf("warning = %#v", data["warning"])
	}
	if warning["type"] != "missing_scope" {
		t.Fatalf("warning.type = %v", warning["type"])
	}
}

func TestWriteLoginSuccess_JSONEmptySlicesNotNull(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	writeLoginSuccess(&LoginOptions{JSON: true}, getLoginMsg("en"), f, "ou_user", "tester", &loginScopeSummary{
		Granted: []string{"offline_access"},
	})

	var data map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	for _, k := range []string{"requested", "newly_granted", "already_granted", "missing", "granted"} {
		v, ok := data[k]
		if !ok {
			t.Fatalf("missing key %q in payload: %v", k, data)
		}
		if _, ok := v.([]interface{}); !ok {
			t.Fatalf("%s = %#v, want JSON array", k, v)
		}
	}
}

func TestWriteLoginSuccess_TextOutputScenarios(t *testing.T) {
	tests := []struct {
		name            string
		summary         *loginScopeSummary
		expectedPresent []string
		expectedAbsent  []string
	}{
		{
			name: "mixed newly granted and already granted",
			summary: &loginScopeSummary{
				Requested:      []string{"im:message:send", "im:message:reply"},
				NewlyGranted:   []string{"im:message:send"},
				AlreadyGranted: []string{"im:message:reply"},
				Granted:        []string{"im:message:send", "im:message:reply"},
			},
			expectedPresent: []string{
				"授权成功! 用户: tester (ou_user)",
				"本次请求 scopes: im:message:send im:message:reply",
				"本次新授予 scopes: im:message:send",
				"可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
			},
			expectedAbsent: []string{
				"本次未授予 scopes:",
				"最终已授权 scopes:",
				"已有 scopes:",
			},
		},
		{
			name: "all already granted",
			summary: &loginScopeSummary{
				Requested:      []string{"im:message:send"},
				AlreadyGranted: []string{"im:message:send"},
				Granted:        []string{"im:message:send", "contact:user.base:readonly"},
			},
			expectedPresent: []string{
				"本次请求 scopes: im:message:send",
				"本次新授予 scopes: （空）",
				"可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
			},
			expectedAbsent: []string{
				"本次未授予 scopes:",
				"最终已授权 scopes:",
				"已有 scopes:",
			},
		},
		{
			name: "missing scopes are shown",
			summary: &loginScopeSummary{
				Requested: []string{"im:message:send", "im:message:reply"},
				Missing:   []string{"im:message:send"},
				Granted:   []string{"im:message:reply"},
			},
			expectedPresent: []string{
				"本次请求 scopes: im:message:send im:message:reply",
				"本次新授予 scopes: （空）",
			},
			expectedAbsent: []string{
				"本次未授予 scopes:",
				"已有 scopes:",
				"最终已授权 scopes:",
				"可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, stderr, _ := cmdutil.TestFactory(t, nil)
			writeLoginSuccess(&LoginOptions{}, getLoginMsg("zh"), f, "ou_user", "tester", tt.summary)

			got := stderr.String()
			for _, want := range tt.expectedPresent {
				if !strings.Contains(got, want) {
					t.Fatalf("stderr missing %q, got:\n%s", want, got)
				}
			}
			for _, unwanted := range tt.expectedAbsent {
				if strings.Contains(got, unwanted) {
					t.Fatalf("stderr should not contain %q, got:\n%s", unwanted, got)
				}
			}
		})
	}
}

func TestBuildLoginScopeSummary_WithMissingScopes(t *testing.T) {
	summary := buildLoginScopeSummary("im:message:send im:message:reply", "im:message:reply", "im:message:reply")
	if got := strings.Join(summary.NewlyGranted, " "); got != "" {
		t.Fatalf("NewlyGranted = %q, want empty", got)
	}
	if got := strings.Join(summary.AlreadyGranted, " "); got != "im:message:reply" {
		t.Fatalf("AlreadyGranted = %q", got)
	}
	if got := strings.Join(summary.Missing, " "); got != "im:message:send" {
		t.Fatalf("Missing = %q", got)
	}
}

func TestAuthLoginRun_MissingRequestedScopeAlignsWithLoginSuccess(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	t.Setenv("HOME", t.TempDir())

	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{Name: "default", AppId: "cli_test"},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, _, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
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
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathOAuthTokenV2,
		Body: map[string]interface{}{
			"access_token":             "user-access-token",
			"refresh_token":            "refresh-token",
			"expires_in":               7200,
			"refresh_token_expires_in": 604800,
			"scope":                    "offline_access",
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    larkauth.PathUserInfoV1,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"open_id": "ou_user",
				"name":    "tester",
			},
		},
	})

	err := authLoginRun(&LoginOptions{
		Factory: f,
		Ctx:     context.Background(),
		Scope:   "im:message:send",
	}, builtinResolver())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if gotCode := output.ExitCodeOf(err); gotCode != output.ExitAuth {
		t.Fatalf("exit code = %d, want %d", gotCode, output.ExitAuth)
	}
	got := stderr.String()
	for _, want := range []string{
		"授权结果异常: 以下请求 scopes 未被授予: im:message:send",
		"当前授权账号: tester (ou_user)",
		"本次请求 scopes: im:message:send",
		"以上结果是本次授权请求用户最终确认后的结果，请勿持续重试",
		"scope 被禁用",
		"lark-cli auth status",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr missing %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "最终已授权 scopes:") {
		t.Fatalf("stderr should not contain final granted scopes, got:\n%s", got)
	}
	if strings.Contains(got, "OK: 授权成功") {
		t.Fatalf("stderr should not contain success prefix when scopes are missing, got:\n%s", got)
	}
	if strings.Contains(got, "本次未授予 scopes:") {
		t.Fatalf("stderr should not duplicate missing scopes, got:\n%s", got)
	}
	if strings.Contains(got, "ERROR:") {
		t.Fatalf("stderr should not contain error prefix, got:\n%s", got)
	}
	stored := larkauth.GetStoredToken("cli_test", "ou_user")
	if stored == nil {
		t.Fatal("expected token to be stored when authorization succeeds with missing scopes")
	}
	if stored.Scope != "offline_access" {
		t.Fatalf("stored scope = %q", stored.Scope)
	}
	cfg, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if len(cfg.Apps) != 1 || len(cfg.Apps[0].Users) != 1 {
		t.Fatalf("unexpected users in config: %#v", cfg.Apps)
	}
	if cfg.Apps[0].Users[0].UserOpenId != "ou_user" {
		t.Fatalf("stored user open id = %q", cfg.Apps[0].Users[0].UserOpenId)
	}
	if cfg.Apps[0].Users[0].UserName != "tester" {
		t.Fatalf("stored user name = %q", cfg.Apps[0].Users[0].UserName)
	}
}

func TestAuthLoginRun_DeviceCodeUsesCachedRequestedScopes(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	t.Setenv("HOME", t.TempDir())

	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{Name: "default", AppId: "cli_test"},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
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
				"open_id": "ou_user",
				"name":    "tester",
			},
		},
	})

	err := authLoginRun(&LoginOptions{
		Factory: f,
		Ctx:     context.Background(),
		Scope:   "im:message:send",
		NoWait:  true,
	}, builtinResolver())
	if err != nil {
		t.Fatalf("no-wait authLoginRun() error = %v", err)
	}
	if got, err := loadLoginRequestedScope("device-code"); err != nil || got != "im:message:send" {
		t.Fatalf("loadLoginRequestedScope() = (%q, %v), want requested scope", got, err)
	}

	stdout.Reset()
	stderr.Reset()

	err = authLoginRun(&LoginOptions{
		Factory:    f,
		Ctx:        context.Background(),
		DeviceCode: "device-code",
	}, builtinResolver())
	if err != nil {
		t.Fatalf("device-code authLoginRun() error = %v", err)
	}
	got := stderr.String()
	for _, want := range []string{
		"OK: 授权成功! 用户: tester (ou_user)",
		"本次请求 scopes: im:message:send",
		"本次新授予 scopes: im:message:send",
		"可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr missing %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "最终已授权 scopes:") {
		t.Fatalf("stderr should not contain final granted scopes, got:\n%s", got)
	}
	if got, err := loadLoginRequestedScope("device-code"); err != nil || got != "" {
		t.Fatalf("loadLoginRequestedScope() after cleanup = (%q, %v), want empty", got, err)
	}
}

func TestWriteLoginSuccess_TextOutputEnglishIncludesStatusHintWhenNoMissingScopes(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)

	writeLoginSuccess(&LoginOptions{}, getLoginMsg("en"), f, "ou_user", "tester", &loginScopeSummary{
		Requested:    []string{"im:message:send"},
		NewlyGranted: []string{"im:message:send"},
		Granted:      []string{"im:message:send"},
	})

	got := stderr.String()
	for _, want := range []string{
		"Authorization successful! User: tester (ou_user)",
		"Requested scopes: im:message:send",
		"Newly granted scopes: im:message:send",
		"Run `lark-cli auth status` to inspect all scopes currently granted to the account.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr missing %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Not granted scopes:") {
		t.Fatalf("stderr should not contain not granted scopes, got:\n%s", got)
	}
}

func TestAuthLoginRun_DeviceCodeTokenNilCleansScopeCache(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)

	if err := saveLoginRequestedScope("device-code", "im:message:send"); err != nil {
		t.Fatalf("saveLoginRequestedScope() error = %v", err)
	}

	original := pollDeviceToken
	t.Cleanup(func() { pollDeviceToken = original })
	pollDeviceToken = func(ctx context.Context, httpClient *http.Client, appId, appSecret string, brand core.LarkBrand, deviceCode string, interval, expiresIn int, errOut io.Writer) *larkauth.DeviceFlowResult {
		return &larkauth.DeviceFlowResult{OK: true, Token: nil}
	}

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})

	err := authLoginRun(&LoginOptions{
		Factory:    f,
		Ctx:        context.Background(),
		DeviceCode: "device-code",
	}, builtinResolver())
	if err == nil {
		t.Fatal("expected error for nil token")
	}
	if !strings.Contains(err.Error(), "authorization succeeded but no token returned") {
		t.Fatalf("error = %v, want nil token error", err)
	}
	if got, err := loadLoginRequestedScope("device-code"); err != nil || got != "" {
		t.Fatalf("loadLoginRequestedScope() after nil token = (%q, %v), want empty", got, err)
	}
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

func TestGetDomainMetadata_ExcludesEvent(t *testing.T) {
	domains := builtinResolver().metadata("zh", "")
	for _, dm := range domains {
		if dm.Name == "event" {
			t.Error("event should not appear in interactive domain list")
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

func TestGetDomainMetadata_ExcludesAuthDomainChildren(t *testing.T) {
	domains := builtinResolver().metadata("zh", "")
	for _, dm := range domains {
		if dm.Name == "whiteboard" {
			t.Error("whiteboard should not appear in interactive domain list (has auth_domain=docs)")
		}
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
