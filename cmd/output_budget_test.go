// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// Output budgets for the surfaces a caller discovers commands through. Each
// ceiling is the measured maximum plus headroom, so an accidental regression
// (a listing that stops being a listing and starts rendering full contracts)
// fails here instead of silently blowing past a consumer's context.
//
// Root help is the one surface whose growth is routine rather than accidental:
// it carries one line per domain (~59 B measured), and onboarding a business
// domain is ordinary work. A ceiling that ordinary work trips stops reporting
// regressions and starts reporting the calendar. 4 KB left 323 B over the
// then-measured 3,773 B — five domains — so it moved to 5 KB, that measurement
// plus at least 15% rounded up to a whole KB (the rule the plan set for the
// per-domain ceiling). The quickstart has since gained the line stating how a
// method path is separated (+126 B, now 3,899 B), still leaving about twenty
// domains. A listing that began rendering full contracts would overshoot by an
// order of magnitude, so the check keeps the failure it was built to catch.
//
// The method index now renders a runnable `command` per row (~63 B measured),
// which took the largest service from 11,736 B to 15,319 B. The 1,065 B left is
// about sixteen more methods on that service — narrower than before, and worth
// knowing, but a different order from what this ceiling guards: the regression
// it exists to catch measured 383,855 B. A service approaching ~73 methods needs
// the ceiling revisited rather than the field dropped.
//
// The rejection envelope is a discovery surface too: a caller reads one every
// time a name does not resolve, and its fallback branch names a whole set of
// subcommands. Every ceiling above watches a success surface, so when that set
// was briefly assembled from the wrong source the root-level envelope grew from
// ~1 KB to 18.6 KB with nothing to catch it. Measured max is 2,994 B (mail);
// 4 KB is that plus 15% rounded up to a whole KB, the same rule the plan set for
// the per-domain ceiling.
//
// That 2,994 B is the conservative reading. An error envelope also carries the
// conditional `identity` and `_notice` fields (internal/output/errors.go), worth
// roughly 600 B together, and `_notice` is rate-limited — so the same command
// measures about 2,400 B on a machine that is not currently due for one. The
// ceiling is set against the larger reading, and the ~600 B of run-to-run
// variation fits inside the headroom.
const (
	maxDomainHelpMedian  = 2560  // 2.5 KB — overall health, resistant to outliers
	maxSingleDomainHelp  = 12288 // 12 KB — measured max 9,965 B (sheets)
	maxRootHelp          = 5120  // 5 KB — measured 3,899 B; see the sizing note above
	maxServiceIndex      = 4096  // 4 KB — measured 1,780 B
	maxMethodIndex       = 16384 // 16 KB — measured max 15,319 B (mail, 57 methods)
	maxRejectionEnvelope = 4096  // 4 KB — measured max 2,994 B (mail); root 1,218 B
)

// buildCLI compiles the real binary once per test run. These budgets are about
// what a caller actually receives, so they assert on the rendered output of the
// built binary rather than on a hand-assembled command tree.
func buildCLI(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lark-cli-budget")
	build := exec.Command("go", "build", "-o", path, ".")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the CLI under test: %v\n%s", err, out)
	}
	return path
}

func runCLI(t *testing.T, cli string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(cli, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	// A non-zero exit with output still tells us the rendered size; only a
	// silent failure is fatal.
	if err := cmd.Run(); err != nil && out.Len() == 0 {
		t.Fatalf("%v: %v\nstderr: %s", args, err, errBuf.String())
	}
	return out.Bytes()
}

// runCLIStderr returns what a caller receives when the command fails: the typed
// envelope goes to stderr, and stdout stays empty. A zero exit means the
// invocation was accepted, which for these probes is itself the failure.
func runCLIStderr(t *testing.T, cli string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(cli, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err == nil {
		t.Fatalf("%v exited 0; expected a rejection\nstdout: %s", args, out.String())
	}
	return errBuf.Bytes()
}

// resourceGroups returns every command path that holds methods without being one,
// derived from the rendered `command` of each index row — the same string a
// caller would copy — so a newly nested resource is covered without anyone
// extending a fixture.
func resourceGroups(t *testing.T, cli string) [][]string {
	t.Helper()
	var index struct {
		Services []struct {
			Name string `json:"name"`
		} `json:"services"`
	}
	if err := json.Unmarshal(runCLI(t, cli, "schema"), &index); err != nil {
		t.Fatalf("service_index is not valid JSON: %v", err)
	}
	seen := map[string]bool{}
	var groups [][]string
	for _, svc := range index.Services {
		var methods struct {
			Methods []struct {
				Command string `json:"command"`
			} `json:"methods"`
		}
		if err := json.Unmarshal(runCLI(t, cli, "schema", svc.Name), &methods); err != nil {
			t.Errorf("method_index for %s is not valid JSON: %v", svc.Name, err)
			continue
		}
		for _, m := range methods.Methods {
			segs := strings.Fields(strings.TrimPrefix(m.Command, "lark-cli "))
			if len(segs) < 3 {
				continue // a method directly under its domain has no resource group
			}
			group := segs[:len(segs)-1]
			if key := strings.Join(group, " "); !seen[key] {
				seen[key] = true
				groups = append(groups, group)
			}
		}
	}
	if len(groups) == 0 {
		t.Fatal("no resource group found, so this probe set cannot detect a regression")
	}
	return groups
}

var sectionHeadRe = regexp.MustCompile(`^[A-Za-z].*:$`)

// domainNames parses the domain list out of root help. Root help groups commands
// under headed sections ("Lark domains:", "Agent tooling:", "CLI management:"),
// and the quickstart above them also uses indented lines — so the walk has to
// stay inside the domain section rather than collecting every indented pair.
func domainNames(t *testing.T, cli string) []string {
	t.Helper()
	var names []string
	inSection := false
	for _, line := range strings.Split(string(runCLI(t, cli, "--help")), "\n") {
		if strings.HasPrefix(line, "Lark domains:") {
			inSection = true
			continue
		}
		if !inSection {
			continue
		}
		if sectionHeadRe.MatchString(line) {
			break // the next section starts; the domain list is done
		}
		fields := strings.Fields(line)
		if !strings.HasPrefix(line, "  ") || len(fields) < 2 {
			continue
		}
		names = append(names, fields[0])
	}
	if len(names) == 0 {
		t.Fatal("could not parse any domain names from root help")
	}
	return names
}

func TestOutputBudget(t *testing.T) {
	cli := buildCLI(t)
	domains := domainNames(t, cli)

	t.Run("HelpSizes", func(t *testing.T) {
		if root := len(runCLI(t, cli, "--help")); root > maxRootHelp {
			t.Errorf("root help is %d bytes, want <= %d", root, maxRootHelp)
		}
		var sizes []int
		for _, d := range domains {
			n := len(runCLI(t, cli, d, "--help"))
			if n > maxSingleDomainHelp {
				t.Errorf("%s help is %d bytes, want <= %d", d, n, maxSingleDomainHelp)
			}
			sizes = append(sizes, n)
		}
		sort.Ints(sizes)
		if median := sizes[len(sizes)/2]; median > maxDomainHelpMedian {
			t.Errorf("domain help median is %d bytes, want <= %d", median, maxDomainHelpMedian)
		}
	})

	t.Run("SchemaSizes", func(t *testing.T) {
		index := runCLI(t, cli, "schema")
		if n := len(index); n > maxServiceIndex {
			t.Errorf("service_index is %d bytes, want <= %d", n, maxServiceIndex)
		}
		// The service index is the authoritative list of services that have a
		// Meta API. Root help is a wider set — it also lists shortcut-only
		// domains, which `schema <domain>` rejects by design — so driving this
		// loop from the index asserts the right set instead of skipping failures.
		var parsed struct {
			Services []struct {
				Name string `json:"name"`
			} `json:"services"`
		}
		if err := json.Unmarshal(index, &parsed); err != nil {
			t.Fatalf("service_index is not valid JSON: %v", err)
		}
		if len(parsed.Services) == 0 {
			t.Fatal("service_index listed no services")
		}
		for _, svc := range parsed.Services {
			out := runCLI(t, cli, "schema", svc.Name)
			if !bytes.Contains(out, []byte(`"method_index"`)) {
				t.Errorf("schema %s did not render a method_index", svc.Name)
				continue
			}
			if len(out) > maxMethodIndex {
				t.Errorf("method_index for %s is %d bytes, want <= %d", svc.Name, len(out), maxMethodIndex)
			}
		}
	})

	// A name that resolves to nothing is answered with the set of names that do,
	// so this envelope grows with the tree exactly as the listings above do — and
	// unlike them it is produced on the error path, where no other check looks.
	t.Run("RejectionEnvelopes", func(t *testing.T) {
		const noSuchName = "zzzzzzzznotasubcommand"
		probes := [][]string{{noSuchName}}
		for _, d := range domains {
			probes = append(probes, []string{d, noSuchName})
		}
		// Resource groups as well. Covering the root and the domains alone is the
		// same shape as the test that let an 18 KB envelope through: named for
		// every level, looping over one of them.
		for _, group := range resourceGroups(t, cli) {
			probes = append(probes, append(append([]string{}, group...), noSuchName))
		}
		for _, args := range probes {
			if n := len(runCLIStderr(t, cli, args...)); n > maxRejectionEnvelope {
				t.Errorf("rejection envelope for %q is %d bytes, want <= %d",
					strings.Join(args, " "), n, maxRejectionEnvelope)
			}
		}
	})

	// ListingLineWidths reports the display-width distribution of domain listing
	// lines (flattened method rows included). It asserts nothing: no truncation
	// rule is in force, and these numbers are what a truncation decision would
	// have to be based on.
	t.Run("ListingLineWidths", func(t *testing.T) {
		var widths []int
		for _, d := range domains {
			for _, line := range strings.Split(string(runCLI(t, cli, d, "--help")), "\n") {
				if !strings.HasPrefix(line, "  ") || strings.TrimSpace(line) == "" {
					continue
				}
				// runewidth, not len(): CJK descriptions occupy two columns each.
				widths = append(widths, runewidth.StringWidth(strings.TrimRight(line, " ")))
			}
		}
		if len(widths) == 0 {
			t.Fatal("no listing lines collected")
		}
		sort.Ints(widths)
		t.Logf("listing line display width: n=%d p50=%d p90=%d max=%d",
			len(widths), widths[len(widths)/2], widths[len(widths)*9/10], widths[len(widths)-1])
	})
}
