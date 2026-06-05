// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd_test

import (
	"strings"
	"testing"
)

func testCatalog() *catalog {
	c := newCatalog()
	c.addCommand("", []string{"--profile"}) // root
	c.setGroup("", true)
	c.addCommand("contact", []string{"--profile"})
	c.setGroup("contact", true)
	c.addCommand("contact +search-user", []string{"--query", "--as", "--format", "-q"})
	c.addCommand("api", []string{"--params", "--data", "--as"}) // leaf (no subcommands)
	c.addCommand("mail", nil)
	c.setGroup("mail", true)
	c.addCommand("mail user_mailbox.messages", []string{"--profile"})
	c.setGroup("mail user_mailbox.messages", true)
	c.addCommand("mail user_mailbox.messages batch_modify", []string{"--params", "--data"})
	return c
}

func TestCmdExampleCatalogHasCommandAndFlag(t *testing.T) {
	c := testCatalog()
	if !c.hasCommand("contact +search-user") {
		t.Fatal("expected contact +search-user to exist")
	}
	if c.hasCommand("contact +nope") {
		t.Fatal("did not expect contact +nope")
	}
	if !c.hasFlag("contact +search-user", "--query") {
		t.Fatal("--query should be valid")
	}
	if c.hasFlag("contact +search-user", "--nope") {
		t.Fatal("--nope should be invalid")
	}
	// universal flags pass on any command
	for _, f := range []string{"--help", "-h", "--version"} {
		if !c.hasFlag("contact +search-user", f) {
			t.Fatalf("universal flag %s should pass", f)
		}
	}
}

func TestCmdExampleLongestPrefix(t *testing.T) {
	c := testCatalog()
	tests := []struct {
		words  []string
		want   string
		wantN  int
		wantOK bool
	}{
		{[]string{"contact", "+search-user"}, "contact +search-user", 2, true},
		{[]string{"api", "GET", "/open-apis/x"}, "api", 1, true}, // trailing positionals
		{[]string{"nope"}, "", 0, false},
		{nil, "", 0, true}, // empty -> root
	}
	for _, tt := range tests {
		got, n, ok := c.longestPrefix(tt.words)
		if got != tt.want || n != tt.wantN || ok != tt.wantOK {
			t.Errorf("longestPrefix(%v) = (%q,%d,%v), want (%q,%d,%v)",
				tt.words, got, n, ok, tt.want, tt.wantN, tt.wantOK)
		}
	}
}

func refWordsOf(refs []ref) [][]string {
	var out [][]string
	for _, r := range refs {
		out = append(out, r.words)
	}
	return out
}

func TestCmdExampleParseRefsExtractsCommands(t *testing.T) {
	content := strings.Join([]string{
		"运行 `lark-cli contact +search-user --query 张三` 搜索", // inline code
		"```bash",
		"lark-cli api GET /open-apis/x --params '{}'", // bash block
		"```",
		"用 lark-cli mail user_mailbox.messages batch_modify 即可", // bare prose command
		"npx foo | lark-cli api GET /y",                         // after a pipe
	}, "\n")
	refs := parseRefs(content)
	if len(refs) != 4 {
		t.Fatalf("expected 4 refs, got %d: %v", len(refs), refWordsOf(refs))
	}
	if got := refs[0]; strings.Join(got.words, " ") != "contact +search-user" ||
		len(got.flags) != 1 || got.flags[0] != "--query" {
		t.Errorf("ref0 = %+v", got)
	}
	if got := refs[1]; strings.Join(got.words, " ") != "api GET /open-apis/x" {
		t.Errorf("ref1 words = %v", got.words)
	}
}

func TestCmdExampleParseRefsFiltersPlaceholdersAndProse(t *testing.T) {
	// A line whose first word is prose yields no command at all.
	if refs := parseRefs("lark-cli 就能搞定这件事"); len(refs) != 0 {
		t.Errorf("prose-first line should yield 0 refs, got %v", refWordsOf(refs))
	}
	// Syntax templates / trailing prose may leave a real leading word ("mail"),
	// but no placeholder or CJK token may leak into the command words — that is
	// what prevents false positives like an "<resource>" unknown-command report.
	for _, line := range []string{
		"lark-cli mail <resource> <method> [flags]",
		"lark-cli apps +<verb> [flags]",
		"lark-cli base +...",
		"lark-cli mail 写信场景下的格式说明",
	} {
		for _, r := range parseRefs(line) {
			for _, w := range r.words {
				if isPlaceholderOrProse(w) {
					t.Errorf("%q: placeholder/prose token %q leaked into words %v", line, w, r.words)
				}
			}
		}
	}
}

func TestCmdExampleParseRefsStripsTrailingJunk(t *testing.T) {
	// frontmatter-style quoted value: the trailing quote must not bleed into the flag
	refs := parseRefs(`cliHelp: "lark-cli contact --help"`)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if len(refs[0].flags) != 1 || refs[0].flags[0] != "--help" {
		t.Errorf("expected flag --help, got %v", refs[0].flags)
	}
	// bare "-" (stdin marker) and "=value" suffix
	refs = parseRefs("lark-cli api GET /x --params={} --data -")
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	flags := strings.Join(refs[0].flags, " ")
	if flags != "--params --data" {
		t.Errorf("expected '--params --data', got %q", flags)
	}
}

func TestCmdExampleCheck(t *testing.T) {
	c := testCatalog()
	tests := []struct {
		name     string
		r        ref
		wantKind string // "" = no finding
		wantPath string
	}{
		{"valid shortcut", ref{words: []string{"contact", "+search-user"}, flags: []string{"--query"}}, "", ""},
		{"valid leaf positional", ref{words: []string{"api", "GET", "/x"}}, "", ""},
		{"unknown top command", ref{words: []string{"nope"}}, unknownCommand, "nope"},
		{"group leftover = unknown subcommand",
			ref{words: []string{"mail", "user_mailbox.messages", "batch_modify_message"}},
			unknownCommand, "mail user_mailbox.messages batch_modify_message"},
		{"unknown flag", ref{words: []string{"contact", "+search-user"}, flags: []string{"--nope"}}, unknownFlag, "contact +search-user"},
		{"universal flag ok", ref{words: []string{"contact", "+search-user"}, flags: []string{"--help"}}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := checkRefs(c, []ref{tt.r})
			if tt.wantKind == "" {
				if len(fs) != 0 {
					t.Fatalf("expected no finding, got %+v", fs)
				}
				return
			}
			if len(fs) != 1 {
				t.Fatalf("expected 1 finding, got %d: %+v", len(fs), fs)
			}
			if fs[0].kind != tt.wantKind || fs[0].path != tt.wantPath {
				t.Errorf("got kind=%s path=%q, want kind=%s path=%q", fs[0].kind, fs[0].path, tt.wantKind, tt.wantPath)
			}
		})
	}
}

func TestCmdExampleCheckSuggestsNearest(t *testing.T) {
	c := testCatalog()
	fs := checkRefs(c, []ref{{words: []string{"mail", "user_mailbox.messages", "batch_modify_message"}}})
	if len(fs) != 1 || fs[0].suggest != "mail user_mailbox.messages batch_modify" {
		t.Fatalf("expected suggestion 'mail user_mailbox.messages batch_modify', got %+v", fs)
	}
}

// TestCmdExampleParseRefsRobustness covers the parser edge cases hardened after
// review: backslash continuation, underscore flags, $(...) substitution, glued
// separators, trailing punctuation, and the "..." placeholder.
func TestCmdExampleParseRefsRobustness(t *testing.T) {
	cases := []struct {
		name, content, wantWords, wantFlags string
		wantRefs                            int
	}{
		{"backslash continuation joins flags",
			"lark-cli contact +search-user \\\n  --query foo \\\n  --as user",
			"contact +search-user", "--query --as", 1},
		{"underscore flag not truncated",
			"lark-cli whiteboard +update --input_format mermaid",
			"whiteboard +update", "--input_format", 1},
		{"command-substitution flags ignored",
			`lark-cli slides x create --data "$(jq -n --arg c '{}')" --as user`,
			"slides x create", "--data --as", 1},
		{"glued separator truncates",
			"lark-cli auth login; echo done",
			"auth login", "", 1},
		{"trailing CJK punctuation stripped",
			"用 lark-cli auth login。",
			"auth login", "", 1},
		{"ellipsis placeholder stays placeholder",
			"lark-cli base +...",
			"base", "", 1},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			refs := parseRefs(tt.content)
			if len(refs) != tt.wantRefs {
				t.Fatalf("refs=%d want %d: %v", len(refs), tt.wantRefs, refWordsOf(refs))
			}
			if tt.wantRefs == 0 {
				return
			}
			if got := strings.Join(refs[0].words, " "); got != tt.wantWords {
				t.Errorf("words=%q want %q", got, tt.wantWords)
			}
			if got := strings.Join(refs[0].flags, " "); got != tt.wantFlags {
				t.Errorf("flags=%q want %q", got, tt.wantFlags)
			}
		})
	}
}
