// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
)

func TestFirstSentence(t *testing.T) {
	cases := []struct{ in, want string }{
		// Upstream descriptions are shaped "<summary>。Identity: <notes>". The
		// identity notes are already exposed as a separate field, so keeping the
		// tail here would only repeat them.
		{"将用户或机器人拉入群聊。Identity: supports `user` and `bot`", "将用户或机器人拉入群聊"},
		{"列出邮件", "列出邮件"},
		{"first line\nsecond line", "first line"},
		{"", ""},
	}
	for _, c := range cases {
		if got := FirstSentence(c.in); got != c.want {
			t.Errorf("FirstSentence(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeIndexDesc_StripsControlAndZeroWidth(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a\x1b[31mred\x1b[0m", "ared"},      // ANSI escapes stripped
		{"tab\there", "tab here"},            // control chars folded to a space
		{"zero​width", "zerowidth"},          // zero-width space removed
		{"line\nbreak", "line break"},        // newline folded, single line out
		{"trail  \t spaces", "trail spaces"}, // space runs collapsed
		{"", ""},
	}
	for _, c := range cases {
		if got := SanitizeIndexDesc(c.in); got != c.want {
			t.Errorf("SanitizeIndexDesc(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Text that visually reorders itself can make a listing row read as something
// other than what it says, so every bidi-control group has to go — not just the
// embedding/override one.
func TestSanitizeIndexDesc_StripsAllBidiControls(t *testing.T) {
	for _, r := range []rune{
		0x202a, 0x202b, 0x202c, 0x202d, 0x202e, // embedding / override
		0x2066, 0x2067, 0x2068, 0x2069, // isolates
		0x061c, // arabic letter mark
		0x200b, 0x200e, 0x200f, 0xfeff,
	} {
		in := "safe" + string(r) + "tail"
		if got := SanitizeIndexDesc(in); got != "safetail" {
			t.Errorf("SanitizeIndexDesc(U+%04X) = %q, want %q", r, got, "safetail")
		}
	}
}

// C1 carries a second set of escape introducers (0x9b is CSI), so folding only
// C0 would leave those usable.
func TestSanitizeIndexDesc_FoldsC1Controls(t *testing.T) {
	for _, r := range []rune{0x80, 0x9b, 0x9f} {
		in := "a" + string(r) + "b"
		if got := SanitizeIndexDesc(in); got != "a b" {
			t.Errorf("SanitizeIndexDesc(U+%04X) = %q, want %q", r, got, "a b")
		}
	}
}

// The identity note is a separate field, so it must never reach a listing line.
// English descriptions run it on with ASCII punctuation instead of 。, which an
// earlier version only cut on.
func TestFirstSentence_CutsIdentityTailInEnglishText(t *testing.T) {
	cases := []struct{ in, want string }{
		{
			"Create a feed group. Identity: `user` only (`user_access_token`).",
			"Create a feed group",
		},
		{
			"批量查询设置 (e.g. `is_muted` mutes messages); up to 10 chats per request. Identity: `user` only",
			"批量查询设置 (e.g. `is_muted` mutes messages); up to 10 chats per request",
		},
		{"将用户移出群聊。Identity: supports `user`", "将用户移出群聊"},
	}
	for _, c := range cases {
		if got := FirstSentence(c.in); got != c.want {
			t.Errorf("FirstSentence(%q) =\n  %q\nwant\n  %q", c.in, got, c.want)
		}
	}
}

// An abbreviation's period must not be mistaken for a sentence end.
func TestFirstSentence_KeepsAbbreviations(t *testing.T) {
	const in = "Filter by type (e.g. group or p2p) and sort the result"
	if got := FirstSentence(in); got != in {
		t.Errorf("FirstSentence must not truncate at an abbreviation: got %q", got)
	}
}

// Upstream appends skill-internal doc references like
// "[Must-read](references/lark-im-reactions.md)" after the identity note. Those
// paths cannot be resolved from CLI output, so none of them may reach a listing
// line — whether they are cut with the identity tail (every current case) or,
// should one ever appear earlier, stripped of its target.
func TestFirstSentence_DropsSkillInternalDocRefs(t *testing.T) {
	cases := []string{
		"Update a feed group. Identity: `user` only (`user_access_token`).[Must-read](references/lark-im-feed-groups.md)",
		"获取消息表情回复。Identity: supports `user` and `bot`.[Must-read](references/lark-im-reactions.md)",
		"Create a group.[Must-read](references/lark-im-feed-groups.md)",
	}
	for _, in := range cases {
		got := FirstSentence(in)
		if strings.Contains(got, "references/") || strings.Contains(got, "](") {
			t.Errorf("skill-internal doc path leaked: FirstSentence(%q) = %q", in, got)
		}
	}
	// The summary itself must survive intact.
	if got := FirstSentence(cases[0]); got != "Update a feed group" {
		t.Errorf("got %q, want %q", got, "Update a feed group")
	}
}

// The index used to state a dot-to-space rule and pin a worked example to it.
// The rule is gone — a reader had to know which surface a string came from
// before they could pick between two of them, and got it wrong every time — so
// each row now carries the runnable form itself. That form is what must match
// the argv the command tree resolves, for every resource shape.
func TestMethodIndexCommandMatchesCommandPath(t *testing.T) {
	// Flat, shallowest, and deepest resource shapes in the catalog.
	refs := []apicatalog.MethodRef{
		{Service: meta.Service{Name: "mail"}, ResourcePath: []string{"user_mailbox.messages"}, Method: meta.Method{Name: "list"}},
		{Service: meta.Service{Name: "mail"}, ResourcePath: []string{"multi_entity"}, Method: meta.Method{Name: "search"}},
		{Service: meta.Service{Name: "drive"}, ResourcePath: []string{"file.comment.reply.reactions"}, Method: meta.Method{Name: "update_reaction"}},
	}
	got := map[string]string{}
	for _, item := range BuildMethodIndex("mail", refs).Methods {
		got[item.Path] = item.Command
	}
	for _, ref := range refs {
		want := commandPrefix + strings.Join(ref.CommandPath(), " ")
		if got[ref.SchemaPath()] != want {
			t.Errorf("command for %q = %q, want %q", ref.SchemaPath(), got[ref.SchemaPath()], want)
		}
	}
	// One method reachable through two contradictory transformation rules is the
	// defect this removed; a hint that teaches one again reintroduces it.
	for _, banned := range []string{"replace the first", "replace the last", "dots with spaces"} {
		if strings.Contains(methodIndexHint, banned) {
			t.Errorf("hint states a transformation rule again (%q): %q", banned, methodIndexHint)
		}
	}
	// The rendered form still has to be pointed at, or the rows go unread.
	if !strings.Contains(methodIndexHint, "`command`") {
		t.Errorf("hint does not name the command field: %q", methodIndexHint)
	}
}

// The index must not carry less than the human-facing help: when there is no
// curated description, the metadata's own description is used.
func TestBuildServiceIndex_FallsBackToMetadataDescription(t *testing.T) {
	idx := BuildServiceIndex(
		[]meta.Service{
			{Name: "curated", Description: "upstream text"},
			{Name: "bare", Description: "attendance record query"},
		},
		func(name string) string {
			if name == "curated" {
				return "curated text"
			}
			return ""
		},
	)
	got := map[string]string{}
	for _, s := range idx.Services {
		got[s.Name] = s.Description
	}
	if got["curated"] != "curated text" {
		t.Errorf("curated description must win, got %q", got["curated"])
	}
	if got["bare"] != "attendance record query" {
		t.Errorf("missing curated text must fall back to metadata, got %q", got["bare"])
	}
}
