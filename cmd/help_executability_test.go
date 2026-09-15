// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/cmd/service"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/spf13/cobra"
)

// A discovery surface that names a command the tree cannot run is worse than one
// that names nothing: the reader (an agent especially) copies the row verbatim
// and spends its turns on a name that never existed. The tests here hold the one
// invariant that prevents the whole class — everything domain help lists must
// run, and a name that does not must say so — over the real command tree, so a
// newly onboarded domain or a newly nested resource is covered without anyone
// remembering to extend a fixture.

// buildExecutabilityTree assembles the full command tree over the embedded
// catalog, with the guards and help func the real binary installs.
func buildExecutabilityTree(t *testing.T) *cobra.Command {
	t.Helper()
	f, _, _ := newStrictModeDefaultFactory(t, "default", core.StrictModeOff)
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	f.APICatalog = snapshot.Catalog()
	root := buildIntegrationRootCmd(t, f)
	installUnknownSubcommandGuard(root)
	installTipsHelpFunc(root, &service.HelpRenderer{})
	return root
}

// domainAPIListings renders every domain's help and returns the method paths each
// one lists, keyed by domain name. Domains with no Meta API surface are omitted.
func domainAPIListings(t *testing.T, root *cobra.Command) map[string][]string {
	t.Helper()
	listings := map[string][]string{}
	for _, domain := range root.Commands() {
		if !(&service.HelpRenderer{}).PrepareDomainHelp(domain) {
			continue
		}
		if !strings.Contains(domain.Long, "API methods (") {
			continue // a shortcut-only domain
		}
		if paths := listedAPIMethodPaths(t, domain.Long); len(paths) > 0 {
			listings[domain.Name()] = paths
		}
	}
	if len(listings) == 0 {
		t.Fatal("no domain listed any API method, so this fixture cannot detect a regression")
	}
	return listings
}

func TestDomainHelpListsOnlyRunnableMethods(t *testing.T) {
	root := buildExecutabilityTree(t)
	for domain, paths := range domainAPIListings(t, root) {
		for _, path := range paths {
			args := append([]string{domain}, strings.Fields(path)...)
			// cobra's Find returns the deepest command it matched plus the
			// leftovers, so resolving without error is not enough: a listing that
			// rendered a non-executable name resolves to the domain itself. The
			// method-schema-path annotation is what makes a command a method leaf.
			target, rest, err := root.Find(args)
			if err != nil {
				t.Errorf("%s help lists %q, which does not resolve: %v", domain, path, err)
				continue
			}
			if len(rest) > 0 {
				t.Errorf("%s help lists %q, but %q is left unresolved — the listed name is not runnable as written",
					domain, path, strings.Join(rest, " "))
				continue
			}
			if target.Annotations["method-schema-path"] == "" {
				t.Errorf("%s help lists %q, which resolves to %q — not a method leaf",
					domain, path, target.CommandPath())
			}
		}
	}
}

func TestDottedMethodNameFailsStructuredOnHelpPath(t *testing.T) {
	root := buildExecutabilityTree(t)
	for domain, paths := range domainAPIListings(t, root) {
		for _, path := range paths {
			dotted := strings.ReplaceAll(path, " ", ".")
			if dotted == path {
				continue // a method directly under the domain has no dotted form
			}
			// The pre-fix behaviour: cobra's ErrHelp path bypassed RunE, so this
			// invocation printed the domain's own help and exited 0, leaving the
			// caller no signal that the name was wrong — while the same name
			// without --help failed structured. Both paths must reject it.
			err := runHelpPath(t, root, domain, dotted)
			if err == nil {
				t.Errorf("`%s %s --help` was accepted; a dotted method name must be rejected on the help path too",
					domain, dotted)
				continue
			}
			assertRejectsWithRewrite(t, err, dotted, path)
		}
	}
}

func TestDottedMethodNameFailsStructuredOnRunPath(t *testing.T) {
	root := buildExecutabilityTree(t)
	for domain, paths := range domainAPIListings(t, root) {
		for _, path := range paths {
			dotted := strings.ReplaceAll(path, " ", ".")
			if dotted == path {
				continue
			}
			domainCmd, _, err := root.Find([]string{domain})
			if err != nil {
				t.Fatalf("domain %q does not resolve: %v", domain, err)
			}
			assertRejectsWithRewrite(t, unknownSubcommandError(domainCmd, dotted), dotted, path)
		}
	}
}

// runHelpPath executes `<domain> <name> --help` against the tree and returns the
// rejection the help func recorded, or nil when help rendered.
func runHelpPath(t *testing.T, root *cobra.Command, domain, name string) error {
	t.Helper()
	pendingHelpRejection = nil
	t.Cleanup(func() { pendingHelpRejection = nil })
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{domain, name, "--help"})
	if err := root.Execute(); err != nil {
		return err
	}
	return pendingHelpRejection
}

// assertRejectsWithRewrite checks the rejection is the typed invalid_argument a
// caller can act on: exit 2, and a machine-readable suggestion holding the
// runnable form rather than prose the caller has to parse.
func assertRejectsWithRewrite(t *testing.T, err error, dotted, want string) {
	t.Helper()
	if code := output.ExitCodeOf(err); code != 2 {
		t.Errorf("rejecting %q: exit code = %d, want 2 (%v)", dotted, code, err)
	}
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("rejecting %q: error = %v, want a *errs.ValidationError", dotted, err)
	}
	if verr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("rejecting %q: subtype = %q, want %q", dotted, verr.Subtype, errs.SubtypeInvalidArgument)
	}
	var suggested []string
	for _, p := range verr.Params {
		if p.Name == dotted {
			suggested = p.Suggestions
		}
	}
	if len(suggested) == 0 {
		t.Errorf("rejecting %q: no suggestion offered; the caller has nothing to retry with", dotted)
		return
	}
	// The rewrite is certain (the tree holds this exact path), so it must be the
	// single suggestion, not one guess among ranked neighbours.
	if len(suggested) != 1 || suggested[0] != want {
		t.Errorf("rejecting %q: suggestions = %v, want exactly [%q]", dotted, suggested, want)
	}
}

// TestSchemaMethodNameIsRunnable pins the two discovery surfaces to each other:
// `schema <dotted-path>` reports a method's name, and that name must be the exact
// argv the command tree accepts. Both derive from the same catalog, so a drift
// here means one of them started composing the path its own way.
func TestSchemaMethodNameIsRunnable(t *testing.T) {
	root := buildExecutabilityTree(t)
	cli := buildCLI(t)
	for domain, paths := range domainAPIListings(t, root) {
		// One method per domain: each case is a process launch, and the property
		// under test is per-surface, not per-method.
		path := paths[0]
		schemaPath := domain + "." + strings.ReplaceAll(path, " ", ".")
		out := runCLI(t, cli, "schema", schemaPath)
		var env struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(out, &env); err != nil {
			t.Errorf("schema %s: output is not JSON: %v\n%s", schemaPath, err, out)
			continue
		}
		want := domain + " " + path
		if env.Name != want {
			t.Errorf("schema %s reports name %q, but the command tree accepts %q", schemaPath, env.Name, want)
		}
	}
}

// TestHelpPathRejectionExitsTwo runs the built binary so the wiring from the help
// func's recorded rejection through Execute to the process exit code is covered
// end to end, not just the recording of it.
func TestHelpPathRejectionExitsTwo(t *testing.T) {
	cli := buildCLI(t)
	root := buildExecutabilityTree(t)
	listings := domainAPIListings(t, root)

	var domain, dotted, runnable string
	for d, paths := range listings {
		for _, p := range paths {
			if strings.Contains(p, " ") {
				domain, dotted, runnable = d, strings.ReplaceAll(p, " ", "."), p
				break
			}
		}
		if dotted != "" {
			break
		}
	}
	if dotted == "" {
		t.Fatal("no domain exposes a method under a resource group, so this fixture cannot exercise the help path")
	}

	cmd := exec.Command(cli, domain, dotted, "--help")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("`lark-cli %s %s --help` exited 0; it must fail structured\nstdout: %s", domain, dotted, stdout.String())
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("running the CLI: %v", err)
	}
	if code := exitErr.ExitCode(); code != 2 {
		t.Errorf("`lark-cli %s %s --help` exit code = %d, want 2\nstderr: %s", domain, dotted, code, stderr.String())
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Subtype string `json:"subtype"`
			Hint    string `json:"hint"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("stderr is not a typed envelope: %v\n%s", err, stderr.String())
	}
	if envelope.OK || envelope.Error.Subtype != string(errs.SubtypeInvalidArgument) {
		t.Errorf("envelope = %+v, want ok=false subtype=%s", envelope, errs.SubtypeInvalidArgument)
	}
	// The hint has to carry the runnable form verbatim; sending the caller back to
	// --help is what made the original silence self-sustaining. Note the runnable
	// form is not the dotted name with its dots swapped for spaces — a resource
	// segment keeps its own dots — so the listing's form is the expectation.
	if !strings.Contains(envelope.Error.Hint, runnable) {
		t.Errorf("hint must name the runnable form %q, got %q", runnable, envelope.Error.Hint)
	}
}

// sortedDomains fixes the order the listings map would otherwise leave to chance,
// so which method a case picks does not vary run to run.
func sortedDomains(listings map[string][]string) []string {
	out := make([]string, 0, len(listings))
	for d := range listings {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// pickDeterminateMethod returns a domain and one of its method paths whose token
// sequence identifies exactly one method in the tree — both in full and with the
// domain prefix dropped. Only such a path can carry an assertion about a
// determinate rewrite: where several methods match, a ranked did-you-mean is the
// correct answer and a confident rewrite would be the defect.
func pickDeterminateMethod(t *testing.T, root *cobra.Command) (domain, path string) {
	t.Helper()
	paths := methodPathsUnder(root)
	listings := domainAPIListings(t, root)
	for _, d := range sortedDomains(listings) {
		for _, p := range listings[d] {
			// A space is needed for a resource segment to exist at all, and a dot
			// inside that segment for the forms to stay distinct: where the resource
			// name has no dots of its own, splitting the dotted path on every dot
			// reproduces the correct argv, and that case asserts nothing.
			if !strings.Contains(p, " ") || !strings.Contains(p, ".") {
				continue
			}
			if _, ok := normalizedPathRewrite(paths, []string{d + " " + p}); !ok {
				continue
			}
			if _, ok := normalizedPathRewrite(paths, []string{p}); !ok {
				continue
			}
			return d, p
		}
	}
	t.Fatal("no method path is unambiguous enough to assert a determinate rewrite")
	return "", ""
}

// One real method, separated every way the evaluation observed a caller separate
// it — plus the two routes through the help subcommand, which used to answer both
// at exit 0. They are one mistake, a separator the tree does not use, so each must
// be rejected structurally and answered with the form that runs.
func TestSeparatorFormsRejectStructurallyWithRunnableSuggestion(t *testing.T) {
	root := buildExecutabilityTree(t)
	cli := buildCLI(t)
	domain, path := pickDeterminateMethod(t, root)
	segs := append([]string{domain}, strings.Fields(path)...)
	dotted := strings.Join(segs, ".")
	runnable := domain + " " + path

	// The suggestion is phrased relative to the command that rejected it: the
	// root names the domain, a domain names only what lives under it.
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{"fully dotted", []string{dotted}, runnable},
		{"partly dotted", []string{strings.Join(segs[:len(segs)-1], "."), segs[len(segs)-1]}, runnable},
		{"domain prefix dropped", []string{strings.Join(segs[1:], ".")}, runnable},
		{"over-split", strings.Split(dotted, "."), path},
		{"one quoted argument", []string{runnable}, runnable},
		{"help, domain resolved", []string{"help", domain, strings.Join(segs[1:], ".")}, path},
		{"help, whole path dotted", []string{"help", dotted}, runnable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertStructuredRejection(t, cli, tc.argv, tc.want)
		})
	}
}

// assertStructuredRejection runs the built binary so the whole path — rejection,
// envelope, process exit code — is covered, and requires the runnable form to
// arrive as a machine-readable suggestion, not only as prose in the hint.
func assertStructuredRejection(t *testing.T, cli string, argv []string, want string) {
	t.Helper()
	cmd := exec.Command(cli, argv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("`lark-cli %s` exited 0; a mis-separated path must fail structured\nstdout: %s",
			strings.Join(argv, " "), stdout.String())
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("running the CLI: %v", err)
	}
	if code := exitErr.ExitCode(); code != 2 {
		t.Errorf("exit code = %d, want 2\nstderr: %s", code, stderr.String())
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Subtype string `json:"subtype"`
			Hint    string `json:"hint"`
			Params  []struct {
				Suggestions []string `json:"suggestions"`
			} `json:"params"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("stderr is not a typed envelope: %v\n%s", err, stderr.String())
	}
	if envelope.OK || envelope.Error.Subtype != string(errs.SubtypeInvalidArgument) {
		t.Errorf("envelope = %+v, want ok=false subtype=%s", envelope, errs.SubtypeInvalidArgument)
	}
	found := false
	for _, p := range envelope.Error.Params {
		for _, s := range p.Suggestions {
			if s == want {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("no suggestion carries the runnable form %q; hint was %q", want, envelope.Error.Hint)
	}
	if !strings.Contains(envelope.Error.Hint, want) {
		t.Errorf("hint must name the runnable form %q, got %q", want, envelope.Error.Hint)
	}
}

// TestSchemaCommandFieldResolvesToItsOwnMethod pins both schema surfaces to the
// command tree. Each renders a `command`, and stripping the binary name from
// either must leave argv the tree resolves to the very method that row is about —
// not its parent group, and not a neighbour. `name` is already pinned by
// TestSchemaMethodNameIsRunnable; the index is the higher-volume copy source,
// since one call returns every method of a service.
func TestSchemaCommandFieldResolvesToItsOwnMethod(t *testing.T) {
	root := buildExecutabilityTree(t)
	cli := buildCLI(t)
	listings := domainAPIListings(t, root)
	for _, domain := range sortedDomains(listings) {
		var index struct {
			Methods []struct {
				Path    string `json:"path"`
				Command string `json:"command"`
			} `json:"methods"`
		}
		if err := json.Unmarshal(runCLI(t, cli, "schema", domain), &index); err != nil {
			t.Errorf("schema %s: output is not JSON: %v", domain, err)
			continue
		}
		if len(index.Methods) == 0 {
			t.Errorf("schema %s listed no methods, so this domain cannot detect a regression", domain)
			continue
		}
		for _, m := range index.Methods {
			assertCommandResolvesTo(t, root, m.Command, m.Path)
		}
		// The detail view has to agree with the index row it came from, or the two
		// surfaces have started composing the path their own way again.
		first := index.Methods[0]
		var detail struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(runCLI(t, cli, "schema", first.Path), &detail); err != nil {
			t.Errorf("schema %s: output is not JSON: %v", first.Path, err)
			continue
		}
		if detail.Command != first.Command {
			t.Errorf("schema %s reports command %q, but its index row says %q", first.Path, detail.Command, first.Command)
		}
	}
}

func assertCommandResolvesTo(t *testing.T, root *cobra.Command, command, schemaPath string) {
	t.Helper()
	argv, ok := strings.CutPrefix(command, "lark-cli ")
	if !ok {
		t.Errorf("command %q does not begin with the binary name", command)
		return
	}
	target, rest, err := root.Find(strings.Fields(argv))
	if err != nil {
		t.Errorf("command %q does not resolve: %v", command, err)
		return
	}
	if len(rest) > 0 {
		t.Errorf("command %q leaves %q unresolved — it is not runnable as written", command, strings.Join(rest, " "))
		return
	}
	if got := target.Annotations["method-schema-path"]; got != schemaPath {
		t.Errorf("command %q resolves to %q, whose schema path is %q, want the method at %q",
			command, target.CommandPath(), got, schemaPath)
	}
}

// The fallback set must name what the group actually holds, at both levels. The
// two sources are complementary and which one is empty flips between them: under
// a resource group the children are the method leaves (availableSubcommandNames
// has them, methodPathsUnder is empty), under a domain the resource groups are
// hidden (the reverse). Listing one source alone produces a confident "the set is
// this" that omits every method of the other — the exact false conclusion this
// hint exists to prevent.
func TestFallbackSetNamesRealMethodsAtEveryLevel(t *testing.T) {
	root := buildExecutabilityTree(t)
	listings := domainAPIListings(t, root)

	// The root's own children are the domains, so the visible names are already
	// the whole answer to "what goes here". Pulling in the nested method paths
	// answers a question one level deeper and costs an order of magnitude — the
	// first version of this test said "every level" but looped over domains only,
	// which is how that regression reached acceptance review.
	t.Run("root", func(t *testing.T) {
		hint, _ := unknownNameGuidance(root, []string{fallbackProbeName})
		assertFallbackShape(t, hint, "root")
		for _, domain := range sortedDomains(listings) {
			if !strings.Contains(hint, domain) {
				t.Errorf("root fallback set omits the domain %q", domain)
			}
			if nested := domain + " " + listings[domain][0]; strings.Contains(hint, nested) {
				t.Errorf("root fallback set names the nested path %q; naming the domain is the answer at this level", nested)
			}
		}
	})

	// A domain's resource groups are hidden by design, so its methods are
	// reachable by name only if this set states them.
	t.Run("domain", func(t *testing.T) {
		for _, domain := range sortedDomains(listings) {
			domainCmd, _, err := root.Find([]string{domain})
			if err != nil {
				t.Fatalf("domain %q does not resolve: %v", domain, err)
			}
			hint, _ := unknownNameGuidance(domainCmd, []string{fallbackProbeName})
			assertFallbackShape(t, hint, domain)
			for _, path := range listings[domain] {
				if !strings.Contains(hint, path) {
					t.Errorf("%s: fallback set omits %q, which `%s --help` lists; hint = %q",
						domain, path, domain, hint)
					break // one report per domain is enough to identify the regression
				}
			}
		}
	})
}

// fallbackProbeName is close to no real subcommand, so the ranked branch cannot
// fire and the fallback set is what answers.
const fallbackProbeName = "zzzzzzzznotasubcommand"

// assertFallbackShape holds what a fallback set owes its reader at any level: it
// must be the fallback branch at all, and it must leave a way to see more than
// bare names.
func assertFallbackShape(t *testing.T, hint, level string) {
	t.Helper()
	if strings.Contains(hint, "did you mean") {
		t.Fatalf("%s: the probe name ranked against real names, so this case tests nothing: %q", level, hint)
	}
	if !strings.Contains(hint, "--help") {
		t.Errorf("%s: fallback hint has no --help pointer, leaving no way to reach the descriptions: %q", level, hint)
	}
}

// A determinate rewrite claims the caller typed a real path with the wrong
// separator. A single token contains no separator at all, so the claim — and the
// explanation attached to it — cannot be true of that input. It matched only
// because one method in the domain happens to end with that word.
func TestSingleTokenDoesNotEarnADeterminateRewrite(t *testing.T) {
	root := buildExecutabilityTree(t)
	paths := methodPathsUnder(root)
	listings := domainAPIListings(t, root)

	for _, domain := range sortedDomains(listings) {
		domainCmd, _, err := root.Find([]string{domain})
		if err != nil {
			t.Fatalf("domain %q does not resolve: %v", domain, err)
		}
		for _, path := range listings[domain] {
			fields := strings.Fields(path)
			leaf := fields[len(fields)-1] // e.g. "update" out of "spreadsheet.sheet.filters update"
			if _, ok := normalizedPathRewrite(paths, []string{leaf}); ok {
				t.Errorf("%s: bare token %q earned a determinate rewrite; only a multi-segment path can", domain, leaf)
			}
			if _, ok := normalizedPathRewrite(methodPathsUnder(domainCmd), []string{leaf}); ok {
				t.Errorf("%s: bare token %q earned a determinate rewrite below the domain", domain, leaf)
			}
		}
	}
}
