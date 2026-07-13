// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/larksuite/cli/internal/meta"
)

// fixtureMD is a minimal affordance source: two methods, each with a lead
// paragraph (use_when) and a fenced example.
const fixtureMD = "# approval\n" +
	"> skill: lark-approval\n\n" +
	"## instances cc\n" +
	"把一个审批实例抄送给指定用户。\n\n" +
	"### Examples\n\n" +
	"**抄送给用户**\n" +
	"```bash\n" +
	"lark-cli approval instances cc --data '{\"instance_code\":\"x\"}'\n" +
	"```\n\n" +
	"## instances get\n" +
	"查询某审批实例详情。\n\n" +
	"### Examples\n\n" +
	"**按 code 查询**\n" +
	"```bash\n" +
	"lark-cli approval instances get --instance-code \"x\"\n" +
	"```\n"

func TestFor(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) }) // SetSource mutates package state; restore for test isolation
	SetSource(fstest.MapFS{"approval.md": &fstest.MapFile{Data: []byte(fixtureMD)}})

	// A seeded method in a seeded service resolves to its overlay.
	raw, ok := For("approval", "instances.cc")
	if !ok {
		t.Fatal(`For("approval","instances.cc") ok=false, want an overlay`)
	}
	var a struct {
		UseWhen  []string `json:"use_when"`
		Examples []struct {
			Command string `json:"command"`
		} `json:"examples"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("overlay is not valid affordance JSON: %v", err)
	}
	if len(a.UseWhen) == 0 || len(a.Examples) == 0 || a.Examples[0].Command == "" {
		t.Errorf("overlay missing use_when/examples: %s", raw)
	}

	// Misses: unknown method in a known service, and an unknown service, both
	// resolve to ok=false (no panic, no error) so callers treat them as "no
	// guidance".
	if _, ok := For("approval", "instances.no_such_method"); ok {
		t.Error("unknown method should be ok=false")
	}
	if _, ok := For("no_such_service", "x.y"); ok {
		t.Error("unknown service should be ok=false")
	}

	// A second lookup of the same service is served from cache (parsed at most
	// once) and stays consistent.
	if _, ok := For("approval", "instances.get"); !ok {
		t.Error("second lookup in a cached service should still resolve")
	}
}

// Non-bullet paragraph lines under any section are preserved as items, not
// dropped (regression: they previously only updated pending, lost without a fence).
func TestParseDomainMD_ParagraphNotDropped(t *testing.T) {
	md := "# d\n\n## foo bar\nwhat it does.\n\n### Tips\n- a bullet\nplain paragraph note.\n\n### See also\nrun [[other cmd]] first.\n"
	got := parseDomainMD([]byte(md), nil) // nil resolver -> space->dot, "foo bar" -> "foo.bar"
	a, ok := got["foo.bar"]
	if !ok {
		t.Fatal("method not parsed")
	}
	if len(a.Tips) != 2 || a.Tips[1] != "plain paragraph note." {
		t.Errorf("Tips paragraph dropped: %v", a.Tips)
	}
	if len(a.Extensions) != 1 || len(a.Extensions[0].Items) != 1 || a.Extensions[0].Items[0] != "run `other cmd` first." {
		t.Errorf("custom-section paragraph not flowed through: %+v", a.Extensions)
	}
}

// The ### Skills section merges with the domain `> skill:` default: domain
// first, then per-command entries, de-duplicated. A command with no ### Skills
// still inherits the domain default.
func TestParseDomainMD_SkillsMerge(t *testing.T) {
	md := "# d\n> skill: lark-d\n\n" +
		"## foo\ndoes foo.\n\n### Skills\n- lark-workflow\n- lark-d\n\n" + // lark-d duplicates the domain default
		"## bar\ndoes bar.\n"
	got := parseDomainMD([]byte(md), nil)

	if a := got["foo"]; len(a.Skills) != 2 || a.Skills[0] != "lark-d" || a.Skills[1] != "lark-workflow" {
		t.Errorf("foo skills = %v, want [lark-d lark-workflow] (domain first, deduped)", a.Skills)
	}
	if a := got["bar"]; len(a.Skills) != 1 || a.Skills[0] != "lark-d" {
		t.Errorf("bar skills = %v, want [lark-d] (domain default inherited)", a.Skills)
	}
}

// A +-prefixed shortcut heading keys verbatim (no space->dot folding), so it
// matches the shortcut command as mounted.
func TestParseDomainMD_ShortcutHeadingVerbatim(t *testing.T) {
	md := "# d\n\n## +create\ncreate via shortcut.\n"
	got := parseDomainMD([]byte(md), nil)
	if _, ok := got["+create"]; !ok {
		t.Errorf("shortcut heading should key as %q; got keys %v", "+create", keysOf(got))
	}
}

func keysOf(m map[string]meta.Affordance) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
