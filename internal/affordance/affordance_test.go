// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"encoding/json"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/larksuite/cli/internal/apicatalog"
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
	prev := Source()
	t.Cleanup(func() { SetSource(prev) }) // SetSource mutates package state; restore for test isolation
	SetSource(fstest.MapFS{"approval.md": &fstest.MapFile{Data: []byte(fixtureMD)}})

	// A seeded method in a seeded service resolves to its overlay.
	catalog := apicatalog.Catalog{}
	raw, ok := For(catalog, "approval", "instances.cc")
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
	if _, ok := For(catalog, "approval", "instances.no_such_method"); ok {
		t.Error("unknown method should be ok=false")
	}
	if _, ok := For(catalog, "no_such_service", "x.y"); ok {
		t.Error("unknown service should be ok=false")
	}

	// A second lookup of the same service is served from cache (parsed at most
	// once) and stays consistent.
	if _, ok := For(catalog, "approval", "instances.get"); !ok {
		t.Error("second lookup in a cached service should still resolve")
	}
	if skill, ok := DomainSkill("approval"); !ok || skill != "lark-approval" {
		t.Errorf("DomainSkill(approval) = %q, %v; want lark-approval, true", skill, ok)
	}
	if skills, ok := DomainSkills("approval"); !ok || !slices.Equal(skills, []string{"lark-approval"}) {
		t.Errorf("DomainSkills(approval) = %v, %v; want [lark-approval], true", skills, ok)
	} else {
		skills[0] = "mutated"
		if cached, _ := DomainSkills("approval"); !slices.Equal(cached, []string{"lark-approval"}) {
			t.Errorf("DomainSkills returned mutable cache storage: %v", cached)
		}
	}
	if skill, ok := DomainSkill("no_such_service"); ok || skill != "" {
		t.Errorf("DomainSkill(no_such_service) = %q, %v; want empty, false", skill, ok)
	}
	if skills, ok := DomainSkills("no_such_service"); ok || skills != nil {
		t.Errorf("DomainSkills(no_such_service) = %v, %v; want nil, false", skills, ok)
	}
}

func TestFor_APICatalogCommandFormResolver(t *testing.T) {
	prev := Source()
	t.Cleanup(func() { SetSource(prev) })
	SetSource(fstest.MapFS{"drive.md": &fstest.MapFile{Data: []byte(
		"# drive\n\n## files list\nList files.\n",
	)}})
	method := meta.FromMap(map[string]interface{}{"id": "file.list", "httpMethod": "GET"})
	service := meta.ServiceFromMap(map[string]interface{}{
		"name": "drive",
		"resources": map[string]interface{}{
			"files": map[string]interface{}{"methods": map[string]interface{}{"list": method}},
		},
	})
	catalog := apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{service})

	if _, ok := For(catalog, "drive", "file.list"); !ok {
		t.Fatal("catalog command form did not resolve to the metadata method ID")
	}
}

func TestResolver_MappingFollowsItsOwnCatalog(t *testing.T) {
	source := fstest.MapFS{"drive.md": &fstest.MapFile{Data: []byte(
		"# drive\n\n## files list\nList files.\n",
	)}}
	catalog := func(methodID string) apicatalog.Catalog {
		service := meta.ServiceFromMap(map[string]interface{}{
			"name": "drive",
			"resources": map[string]interface{}{
				"files": map[string]interface{}{"methods": map[string]interface{}{
					"list": map[string]interface{}{"id": methodID, "httpMethod": "GET"},
				}},
			},
		})
		return apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{service})
	}
	first := NewResolver(source, catalog("file.list"))
	second := NewResolver(source, catalog("file.list.v2"))

	if _, ok := first.For("drive", "file.list"); !ok {
		t.Fatal("first resolver mapping did not resolve")
	}
	if _, ok := second.For("drive", "file.list.v2"); !ok {
		t.Fatal("second resolver did not apply its own catalog mapping")
	}
	if _, ok := second.For("drive", "file.list"); ok {
		t.Fatal("second resolver exposed an overlay keyed by the first catalog")
	}
}

// countingFS records how often each file is opened so the test can pin that a
// Resolver reads and maps a service exactly once, including a missing file.
type countingFS struct {
	fstest.MapFS
	opens map[string]int
}

func (c *countingFS) Open(name string) (fs.File, error) {
	c.opens[name]++
	return c.MapFS.Open(name)
}

// ReadFile shadows MapFS's fast path so fs.ReadFile is counted too.
func (c *countingFS) ReadFile(name string) ([]byte, error) {
	c.opens[name]++
	return c.MapFS.ReadFile(name)
}

func TestResolver_ReadsAndMapsEachServiceOnce(t *testing.T) {
	source := &countingFS{
		MapFS: fstest.MapFS{"drive.md": &fstest.MapFile{Data: []byte(
			"# drive\n> skill: lark-drive\n\n## files list\nList files.\n\n## files get\nGet.\n",
		)}},
		opens: map[string]int{},
	}
	service := meta.ServiceFromMap(map[string]interface{}{
		"name": "drive",
		"resources": map[string]interface{}{
			"files": map[string]interface{}{"methods": map[string]interface{}{
				"list": map[string]interface{}{"id": "file.list", "httpMethod": "GET"},
				"get":  map[string]interface{}{"id": "file.get", "httpMethod": "GET"},
			}},
		},
	})
	r := NewResolver(source, apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{service}))

	for _, id := range []string{"file.list", "file.get", "file.list", "file.missing"} {
		r.For("drive", id)
	}
	r.DomainSkill("drive")
	r.DomainSkills("drive")
	if got := source.opens["drive.md"]; got != 1 {
		t.Fatalf("drive.md opened %d times, want 1", got)
	}

	for range 3 {
		if _, ok := r.For("calendar", "x.y"); ok {
			t.Fatal("service without guidance must report no overlay")
		}
	}
	if got := source.opens["calendar.md"]; got != 1 {
		t.Fatalf("missing calendar.md probed %d times, want 1 (absence is cached)", got)
	}
}

func TestResolver_NilAndSourcelessAreSilent(t *testing.T) {
	var nilResolver *Resolver
	if _, ok := nilResolver.For("drive", "file.list"); ok {
		t.Fatal("nil Resolver must report no guidance")
	}
	if _, ok := nilResolver.DomainSkills("drive"); ok {
		t.Fatal("nil Resolver must report no skills")
	}
	r := NewResolver(nil, apicatalog.Catalog{})
	if _, ok := r.For("drive", "file.list"); ok {
		t.Fatal("Resolver without a source must report no guidance")
	}
	if _, ok := r.DomainSkill("drive"); ok {
		t.Fatal("Resolver without a source must report no skill")
	}
}

// Non-bullet paragraph lines under any section are preserved as items, not
// dropped (regression: they previously only updated pending, lost without a fence).
func TestParseDomainMD_ParagraphNotDropped(t *testing.T) {
	md := "# d\n\n## foo bar\nwhat it does.\n\n### Tips\n- a bullet\nplain paragraph note.\n\n### See also\nrun [[other cmd]] first.\n"
	got := parseDomainMD([]byte(md), nil) // nil resolver -> space->dot, "foo bar" -> "foo.bar"
	a, ok := got.methods["foo.bar"]
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

	if got.skill != "lark-d" {
		t.Errorf("domain skill = %q, want lark-d", got.skill)
	}
	if a := got.methods["foo"]; len(a.Skills) != 2 || a.Skills[0] != "lark-d" || a.Skills[1] != "lark-workflow" {
		t.Errorf("foo skills = %v, want [lark-d lark-workflow] (domain first, deduped)", a.Skills)
	}
	if a := got.methods["bar"]; len(a.Skills) != 1 || a.Skills[0] != "lark-d" {
		t.Errorf("bar skills = %v, want [lark-d] (domain default inherited)", a.Skills)
	}
}

// The reserved domain-level ## Skills section controls domain-help navigation
// only. The canonical > skill: remains first and is still the sole default
// inherited by commands.
func TestParseDomainMD_DomainSkills(t *testing.T) {
	md := "# d\n> skill: lark-d\n\n" +
		"## Skills  \n- lark-workflow\n- `lark-d`\n- lark-shared\n\n" +
		"## foo\ndoes foo.\n\n### Skills\n- lark-command\n"
	got := parseDomainMD([]byte(md), nil)

	if want := []string{"lark-d", "lark-workflow", "lark-shared"}; !slices.Equal(got.domainSkills, want) {
		t.Errorf("domain skills = %v, want %v", got.domainSkills, want)
	}
	if want := []string{"lark-d", "lark-command"}; !slices.Equal(got.methods["foo"].Skills, want) {
		t.Errorf("foo skills = %v, want %v; domain-only skills must not leak into commands", got.methods["foo"].Skills, want)
	}
}

// A +-prefixed shortcut heading keys verbatim (no space->dot folding), so it
// matches the shortcut command as mounted.
func TestParseDomainMD_ShortcutHeadingVerbatim(t *testing.T) {
	md := "# d\n\n## +create\ncreate via shortcut.\n"
	got := parseDomainMD([]byte(md), nil)
	if _, ok := got.methods["+create"]; !ok {
		t.Errorf("shortcut heading should key as %q; got keys %v", "+create", keysOf(got.methods))
	}
}

func keysOf(m map[string]meta.Affordance) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
