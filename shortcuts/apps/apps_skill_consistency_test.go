// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// frameworkGlobalFlags are injected by shortcuts/common/runner.go for every (or
// many) shortcuts, so they are always allowed in skill docs regardless of which
// command they are attached to. See registerShortcutFlagsWithContext in
// shortcuts/common/runner.go: --dry-run, --format, --json, --jq/-q are injected
// unconditionally; --as via the identity flag; --yes for high-risk-write;
// --print-schema/--flag-name for shortcuts that opt into schema introspection;
// --help/-h are cobra built-ins.
var frameworkGlobalFlags = map[string]bool{
	"dry-run": true, "format": true, "json": true, "yes": true,
	"jq": true, "q": true, "as": true,
	"print-schema": true, "flag-name": true, "help": true, "h": true,
}

// cmdRef is one apps command invocation extracted from a skill doc.
type cmdRef struct {
	cmd   string   // registered command form, includes the leading '+'
	flags []string // long flag names without '--', short flags without '-'
}

var (
	// cmdTokenRe matches a shortcut command token. The leading '+' is the
	// reliable signal; the body is a-z plus digits/hyphens. A real command
	// never ends in '-', so a trailing hyphen (from a glob like `+db-*`) is
	// stripped/rejected separately.
	cmdTokenRe  = regexp.MustCompile(`\+[a-z][a-z0-9-]*`)
	longFlagRe  = regexp.MustCompile(`^--([a-z][a-z0-9-]*)`)
	shortFlagRe = regexp.MustCompile(`^-([a-z])$`)
	// bareWordRe matches a plain lowercase word (a CLI service/qualifier token
	// like "apps", "contact", "im", "code") with no markdown decoration.
	bareWordRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// documentedNonexistentExamples are apps-prefixed command tokens the lark-apps
// docs deliberately cite as NOT being apps commands (negative examples that
// warn agents away from inventing them). They are intentionally absent from
// Shortcuts(); excluding them here is narrow and explicit, unlike skipping any
// line containing "不存在" (a common Chinese word meaning "does not exist")
// which would mask real drift on unrelated lines.
//
// Source lines (both files carry the identical sentence):
//   - skills/lark-apps/references/lark-apps-local-dev.md:52
//   - skills/lark-apps/references/lark-apps-git-credential.md:35
//     "...不存在 `apps +pull` / `apps +push` / `apps code +read` 这类...shortcut..."
//
// Only `+pull` and `+push` are apps-prefixed and thus need an explicit entry
// here. `apps code +read` is preceded by the bare qualifier word "code" (not
// "apps"), so the cross-service filter already rejects it; adding it would be
// redundant double-coverage, so it is intentionally omitted.
var documentedNonexistentExamples = map[string]bool{
	"+pull": true,
	"+push": true,
}

// extractCmdRefs joins backslash-continued lines, then for each `+<cmd>` token
// captures the --flags/-q that follow it, stopping at the next `+<cmd>` token, a
// shell separator (| && ;), or the end of the inline-code span the command
// appears in. Flags only attach within the same backtick-delimited segment as
// the command, because skill docs write a real invocation inside one code span
// (`lark-cli apps +create --name x`) while a stray `--flag` discussed in prose
// (e.g. "`+git-credential-list` ... 不需要 `--app-id`") lives in a separate
// span and must not attach.
//
// To avoid false positives it also:
//   - skips a `+token` immediately preceded by a bare service/qualifier word
//     other than "apps" (e.g. `contact +search-user`, `im +chat-search`,
//     `apps code +read`) — those are not apps shortcuts;
//   - rejects a token that ends in '-' (a wildcard family like `+db-*`,
//     `+release-*`), since no registered command ends in a hyphen.
//
// Deliberate negative examples (documentedNonexistentExamples, e.g. `+pull`)
// are still extracted here; the consistency gate skips them explicitly when an
// unregistered command turns out to be one of those documented examples.
func extractCmdRefs(doc string) []cmdRef {
	var refs []cmdRef
	for _, logical := range logicalLines(doc) {
		// Split the logical line into backtick-delimited segments. A command and
		// its flags only travel together within one segment; crossing a backtick
		// boundary resets the capture context. Code-block lines (no backticks)
		// are a single segment and behave like a normal command line.
		var cur *cmdRef
		var prevClean string
		for _, seg := range strings.Split(logical, "`") {
			cur = nil // a new inline span never inherits the previous command
			prevClean = ""
			for _, tok := range strings.Fields(seg) {
				clean := strings.Trim(tok, ",'\"()*")
				if tok == "|" || tok == "&&" || tok == ";" {
					cur = nil
					prevClean = clean
					continue
				}
				if strings.HasPrefix(clean, "+") {
					m := cmdTokenRe.FindString(clean)
					if m == "" || strings.HasSuffix(m, "-") {
						// Not a real command shape (e.g. "+1") or a wildcard
						// family like "+db-" from `+db-*`. No capture context.
						cur = nil
						prevClean = clean
						continue
					}
					// Cross-service reference: nearest preceding bare word is a
					// service/qualifier other than "apps".
					if bareWordRe.MatchString(prevClean) && prevClean != "apps" && prevClean != "lark-cli" {
						cur = nil
						prevClean = clean
						continue
					}
					refs = append(refs, cmdRef{cmd: m})
					cur = &refs[len(refs)-1]
					prevClean = clean
					continue
				}
				if cur != nil {
					if m := longFlagRe.FindStringSubmatch(clean); m != nil {
						cur.flags = append(cur.flags, m[1])
					} else if m := shortFlagRe.FindStringSubmatch(clean); m != nil {
						cur.flags = append(cur.flags, m[1])
					}
				}
				prevClean = clean
			}
		}
	}
	return refs
}

// logicalLines merges lines ending with a backslash into one logical line.
func logicalLines(doc string) []string {
	raw := strings.Split(strings.ReplaceAll(doc, "\r\n", "\n"), "\n")
	var out []string
	var buf strings.Builder
	carrying := false
	for _, ln := range raw {
		t := strings.TrimRight(ln, " \t")
		if strings.HasSuffix(t, "\\") {
			buf.WriteString(strings.TrimSuffix(t, "\\"))
			buf.WriteString(" ")
			carrying = true
			continue
		}
		buf.WriteString(ln)
		out = append(out, buf.String())
		buf.Reset()
		carrying = false
	}
	if carrying || buf.Len() > 0 {
		out = append(out, buf.String())
	}
	return out
}

func TestExtractCmdRefs_Unit(t *testing.T) {
	doc := "`lark-cli apps +create --name x --app-type html`\n" +
		"`+db-table-list`, `+db-table-get`\n" +
		"lark-cli apps +session-list --app-id x | jq '.y --post-pipe-flag'\n" +
		"lark-cli apps +foo --bar baz \\\n  --qux 1\n" +
		"人名→`ou_` 用 `lark-cli contact +search-user --query <名字>`，群名→`oc_` 用 `lark-cli im +chat-search --query <群名>`\n" +
		"改库走 `+db-*`；发布走 `+release-*`\n" +
		"不存在 `apps +pull` / `apps +push` / `apps code +read` 这类 shortcut，不要臆造。\n" +
		"`+git-credential-list` 列出本地凭证，不需要 `--app-id`。\n"

	refs := extractCmdRefs(doc)
	got := map[string][]string{}
	for _, r := range refs {
		got[r.cmd] = append(got[r.cmd], r.flags...)
	}

	// Full invocation: command + both flags captured.
	if _, ok := got["+create"]; !ok {
		t.Fatalf("missing +create; got %+v", refs)
	}
	if !contains(got["+create"], "name") || !contains(got["+create"], "app-type") {
		t.Errorf("+create flags wrong: %v", got["+create"])
	}

	// Comma-separated command list: no flags attach to either command.
	if _, ok := got["+db-table-list"]; !ok {
		t.Errorf("missing +db-table-list; got %+v", refs)
	}
	if len(got["+db-table-list"]) != 0 || len(got["+db-table-get"]) != 0 {
		t.Errorf("comma-separated commands must carry no flags: %v", got)
	}

	// Pipe stops capture within a SINGLE span (no surrounding backticks), so the
	// pipe `|` is the only boundary that can stop flag capture here: --app-id
	// (before the pipe) attaches, but the post-pipe --post-pipe-flag must NOT.
	if !contains(got["+session-list"], "app-id") {
		t.Errorf("pre-pipe flag should attach to +session-list: %v", got["+session-list"])
	}
	if contains(got["+session-list"], "post-pipe-flag") {
		t.Errorf("pipe did not stop flag capture: %v", got["+session-list"])
	}

	// Backslash continuation joins --qux onto +foo (same logical line).
	if !contains(got["+foo"], "bar") || !contains(got["+foo"], "qux") {
		t.Errorf("continuation join failed: %v", got["+foo"])
	}

	// Cross-service commands must NOT be attributed to apps.
	if _, ok := got["+search-user"]; ok {
		t.Errorf("contact +search-user must not be extracted as an apps command: %+v", refs)
	}
	if _, ok := got["+chat-search"]; ok {
		t.Errorf("im +chat-search must not be extracted as an apps command: %+v", refs)
	}

	// Wildcard family references must NOT be extracted as commands.
	if _, ok := got["+db-"]; ok {
		t.Errorf("`+db-*` wildcard must not be extracted as a command: %+v", refs)
	}
	if _, ok := got["+release-"]; ok {
		t.Errorf("`+release-*` wildcard must not be extracted as a command: %+v", refs)
	}

	// Deliberate negative examples are no longer line-skipped: the apps-prefixed
	// `+pull` / `+push` ARE extracted here (the consistency gate later excludes
	// them via documentedNonexistentExamples). `apps code +read` is preceded by
	// the bare qualifier "code", so the cross-service filter still drops it.
	for _, tok := range []string{"+pull", "+push"} {
		if _, ok := got[tok]; !ok {
			t.Errorf("negative example %s should still be extracted (gate excludes it, not the extractor): %+v", tok, refs)
		}
		if !documentedNonexistentExamples[tok] {
			t.Errorf("%s must be in documentedNonexistentExamples allowlist", tok)
		}
	}
	if _, ok := got["+read"]; ok {
		t.Errorf("`apps code +read` is cross-service (preceded by `code`) and must not be extracted: %+v", refs)
	}

	// A --flag discussed in prose, in a separate inline-code span from the
	// command, must NOT attach to the command (backtick-span boundary stops
	// capture).
	if contains(got["+git-credential-list"], "app-id") {
		t.Errorf("prose flag in a separate backtick span must not attach: %v", got["+git-credential-list"])
	}
}

func TestSkillDocsCommandsConsistentWithShortcuts(t *testing.T) {
	// Source of truth: the registered shortcuts and their flags.
	validCmd := map[string]map[string]bool{}
	for _, s := range Shortcuts() {
		fl := map[string]bool{}
		for _, f := range s.Flags {
			fl[f.Name] = true
		}
		validCmd[s.Command] = fl
	}

	docs := skillDocFiles(t)
	if len(docs) == 0 {
		t.Fatal("no lark-apps skill docs found; gate cannot run")
	}

	for _, path := range docs {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		rel := filepath.Base(path)
		for _, ref := range extractCmdRefs(string(raw)) {
			flags, ok := validCmd[ref.cmd]
			if !ok {
				// A deliberate negative example (documented as NOT existing) is
				// expected to be absent from Shortcuts(); skip only those.
				if documentedNonexistentExamples[ref.cmd] {
					continue
				}
				t.Errorf("%s: references `apps %s` which is not a registered shortcut", rel, ref.cmd)
				continue
			}
			for _, fl := range ref.flags {
				if flags[fl] || frameworkGlobalFlags[fl] {
					continue
				}
				t.Errorf("%s: `apps %s --%s`: --%s is not a flag of %s (have: %s)",
					rel, ref.cmd, fl, fl, ref.cmd, sortedFlags(flags))
			}
		}
	}
}

// skillDocFiles returns SKILL.md + references/*.md for lark-apps, relative to
// this package dir (go test cwd = shortcuts/apps/).
func skillDocFiles(t *testing.T) []string {
	t.Helper()
	base := filepath.Join("..", "..", "skills", "lark-apps")
	var out []string
	if _, err := os.Stat(filepath.Join(base, "SKILL.md")); err == nil {
		out = append(out, filepath.Join(base, "SKILL.md"))
	}
	refs, _ := filepath.Glob(filepath.Join(base, "references", "*.md"))
	out = append(out, refs...)
	return out
}

func sortedFlags(m map[string]bool) string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
