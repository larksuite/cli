// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/selfupdate"
)

func TestParseSkillsListIgnoresUnsupportedFormat(t *testing.T) {
	input := `Installed skills:
- lark-calendar
- lark-mail
lark-im
custom-skill
lark-base@1.0.0
lark-cli-harness:dev@0.1.0
`
	got := ParseSkillsList(input)
	if len(got) != 0 {
		t.Fatalf("ParseSkillsList() = %#v, want empty result for unsupported format", got)
	}
}

func TestParseGlobalSkillsList(t *testing.T) {
	input := `Global Skills

lark-approval ~/.agents/skills/lark-approval
  Agents: TRAE CN, TRAE, TRAE-SOLO, TRAE CLI, TRAE CLI (Coco) +3 more
lark-attendance ~/.agents/skills/lark-attendance
  Agents: TRAE CN, TRAE, TRAE-SOLO, TRAE CLI, TRAE CLI (Coco) +3 more
lark-base ~/.agents/skills/lark-base
  Agents: TRAE CN, TRAE, TRAE-SOLO, TRAE CLI, TRAE CLI (Coco) +3 more
lark-calendar ~/.agents/skills/lark-calendar
  Agents: TRAE CN, TRAE, TRAE-SOLO, TRAE CLI, TRAE CLI (Coco) +3 more
dogfood ~/.hermes/skills/dogfood
  Agents: Hermes Agent
yuanbao ~/.hermes/skills/yuanbao
  Agents: Hermes Agent
`
	got := ParseSkillsList(input)
	want := []string{"dogfood", "lark-approval", "lark-attendance", "lark-base", "lark-calendar", "yuanbao"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSkillsList() (Global Skills) = %#v, want %#v", got, want)
	}
}

func TestParseGlobalSkillsListWithANSI(t *testing.T) {
	input := "\x1b[1mGlobal Skills\x1b[0m\n\n" +
		"\x1b[36mlark-calendar\x1b[0m \x1b[38;5;102m~/.agents/skills/lark-calendar\x1b[0m\n" +
		"  \x1b[38;5;102mAgents:\x1b[0m TRAE CN, TRAE +3 more\n" +
		"\x1b[36mdogfood\x1b[0m \x1b[38;5;102m~/.hermes/skills/dogfood\x1b[0m\n" +
		"  \x1b[38;5;102mAgents:\x1b[0m Hermes Agent\n" +
		"\nTip: Use the -y flag to run in non-interactive mode (for CI and AI agents).\n"
	got := ParseSkillsList(input)
	want := []string{"dogfood", "lark-calendar"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSkillsList() (ANSI Global Skills) = %#v, want %#v", got, want)
	}
}

func TestParseGlobalSkillsListWithIndentedGroupedRows(t *testing.T) {
	input := `Global Skills

General
  lark-apps ~/.agents/skills/lark-apps
  lark-base ~/.agents/skills/lark-base
`
	got := ParseSkillsList(input)
	want := []string{"lark-apps", "lark-base"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSkillsList() (indented Global Skills) = %#v, want %#v", got, want)
	}
}

func TestParseOfficialSkillsIndexJSON(t *testing.T) {
	input := `{
	  "skills": [
	    {"name":"lark-calendar","type":"archive","url":"./lark-calendar.tar.gz","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	    {"name":"lark-mail","type":"archive","url":"./lark-mail.tar.gz","digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	  ]
	}`
	got, err := ParseOfficialSkillsIndexJSON(input)
	if err != nil {
		t.Fatalf("ParseOfficialSkillsIndexJSON() err = %v, want nil", err)
	}
	want := []string{"lark-calendar", "lark-mail"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseOfficialSkillsIndexJSON() = %#v, want %#v", got, want)
	}
}

func TestParseOfficialSkillsIndexJSONInvalidOrUnsupported(t *testing.T) {
	for _, input := range []string{
		`not json`,
		`[{"name":"lark-calendar"}]`,
		`{"name":"lark-calendar"}`,
		`{"skills":[]}`,
		`{"skills":[{"name":"bad skill","type":"archive","url":"./x.tar.gz","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`,
		`{"skills":[{"name":"lark-mail","type":"archive","url":"./x.tar.gz","digest":"bad"}]}`,
	} {
		got, err := ParseOfficialSkillsIndexJSON(input)
		if err == nil && len(got) != 0 {
			t.Fatalf("ParseOfficialSkillsIndexJSON(%q) = %#v, want empty", input, got)
		}
	}
}

func TestPlanNormal_WithReadableStatePreservesDeletedAndAddsNew(t *testing.T) {
	previous := &SkillsState{OfficialSkills: []string{"lark-calendar", "lark-mail"}}
	got := PlanSync(SyncInput{
		Version:        "1.0.33",
		OfficialSkills: []string{"lark-calendar", "lark-mail", "lark-new"},
		LocalSkills:    []string{"lark-calendar", "lark-custom"},
		PreviousState:  previous,
		StateReadable:  true,
		Force:          false,
	})

	assertStrings(t, got.ToUpdate, []string{"lark-calendar", "lark-new"})
	assertStrings(t, got.Added, []string{"lark-new"})
	assertStrings(t, got.SkippedDeleted, []string{"lark-mail"})
}

func TestPlanRestoresAllWhenNoOfficialSkillsRemain(t *testing.T) {
	for _, test := range []struct {
		name      string
		official  []string
		wantAdded []string
	}{
		{name: "unchanged official set", official: []string{"lark-calendar", "lark-mail"}, wantAdded: []string{}},
		{name: "new official skill", official: []string{"lark-calendar", "lark-mail", "lark-new"}, wantAdded: []string{"lark-new"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := PlanSync(SyncInput{
				Version:        "1.0.33",
				OfficialSkills: test.official,
				LocalSkills:    []string{"custom-skill"},
				PreviousState:  &SkillsState{OfficialSkills: []string{"lark-calendar", "lark-mail"}},
				StateReadable:  true,
			})

			assertStrings(t, got.ToUpdate, test.official)
			assertStrings(t, got.Added, test.wantAdded)
			assertStrings(t, got.SkippedDeleted, []string{})
		})
	}
}

func TestPlanNormal_MissingStateInstallsAllOfficial(t *testing.T) {
	got := PlanSync(SyncInput{
		Version:        "1.0.33",
		OfficialSkills: []string{"lark-calendar", "lark-mail", "lark-new"},
		LocalSkills:    []string{"lark-calendar"},
		StateReadable:  false,
		Force:          false,
	})

	assertStrings(t, got.ToUpdate, []string{"lark-calendar", "lark-mail", "lark-new"})
	assertStrings(t, got.Added, []string{"lark-calendar", "lark-mail", "lark-new"})
	assertStrings(t, got.SkippedDeleted, []string{})
}

func TestPlanForceRestoresAllOfficial(t *testing.T) {
	got := PlanSync(SyncInput{
		Version:        "1.0.33",
		OfficialSkills: []string{"lark-calendar", "lark-mail", "lark-new"},
		LocalSkills:    []string{"lark-calendar"},
		PreviousState:  &SkillsState{OfficialSkills: []string{"lark-calendar", "lark-mail"}},
		StateReadable:  true,
		Force:          true,
	})

	assertStrings(t, got.ToUpdate, []string{"lark-calendar", "lark-mail", "lark-new"})
	assertStrings(t, got.Added, []string{})
	assertStrings(t, got.SkippedDeleted, []string{})
}

type fakeSkillsRunner struct {
	sources       []string
	indexes       map[string]string
	indexErrors   map[string]error
	installErrors map[string]error
	stageErrors   map[string]error
	stageChildren map[string][]string
	globalJSON    string
	installs      []string
	removals      [][]string
	localSuite    string
	stages        []string
}

func officialSkillsIndexOutput(names ...string) string {
	var b strings.Builder
	b.WriteString(`{"skills":[`)
	for i, name := range names {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"name":%q,"description":"test","type":"archive","url":"./%s.tar.gz","digest":"sha256:%s"}`, name, name, strings.Repeat("a", 64))
	}
	b.WriteString(`]}`)
	return b.String()
}

func globalSkillsJSONOutput(names ...string) string {
	var b strings.Builder
	b.WriteString("[")
	for i, name := range names {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"name":%q,"path":"/Users/example/.agents/skills/%s","scope":"global"}`, name, name)
	}
	b.WriteString("]")
	return b.String()
}

func (f *fakeSkillsRunner) SkillsSources() []string { return f.sources }
func (f *fakeSkillsRunner) FetchSkillsIndex(source string) *selfupdate.NpmResult {
	r := &selfupdate.NpmResult{}
	r.Stdout.WriteString(f.indexes[source])
	r.Err = f.indexErrors[source]
	return r
}
func (f *fakeSkillsRunner) ListGlobalSkillsJSON() *selfupdate.NpmResult {
	r := &selfupdate.NpmResult{}
	r.Stdout.WriteString(f.globalJSON)
	return r
}
func (f *fakeSkillsRunner) ListGlobalSkills() *selfupdate.NpmResult {
	return &selfupdate.NpmResult{Err: fmt.Errorf("unexpected text fallback")}
}
func (f *fakeSkillsRunner) InstallSkills(source string, names []string) *selfupdate.NpmResult {
	f.installs = append(f.installs, source+":"+strings.Join(names, ","))
	return &selfupdate.NpmResult{Err: f.installErrors[source]}
}
func (f *fakeSkillsRunner) InstallAllSkills(source string) *selfupdate.NpmResult {
	f.installs = append(f.installs, source+":*")
	return &selfupdate.NpmResult{Err: f.installErrors[source]}
}
func (f *fakeSkillsRunner) StageSuite(source, dir string) *selfupdate.NpmResult {
	f.stages = append(f.stages, source)
	if err := f.stageErrors[source]; err != nil {
		return &selfupdate.NpmResult{Err: err}
	}
	suite := filepath.Join(dir, ".agents", "skills", "lark-suite")
	children := f.stageChildren[source]
	if children == nil {
		children = []string{"lark-calendar", "lark-mail"}
	}
	for _, name := range children {
		if err := os.MkdirAll(filepath.Join(suite, "references", name), 0o755); err != nil {
			return &selfupdate.NpmResult{Err: err}
		}
	}
	if err := os.WriteFile(filepath.Join(suite, "SKILL.md"), []byte(suiteFixture), 0o644); err != nil {
		return &selfupdate.NpmResult{Err: err}
	}
	return &selfupdate.NpmResult{}
}
func (f *fakeSkillsRunner) InstallLocalSuite(path string) *selfupdate.NpmResult {
	f.localSuite = path
	return &selfupdate.NpmResult{}
}
func (f *fakeSkillsRunner) RemoveGlobalSkills(names []string) *selfupdate.NpmResult {
	f.removals = append(f.removals, append([]string{}, names...))
	return &selfupdate.NpmResult{}
}

const suiteFixture = `---
name: lark-suite
description: 飞书/Lark 聚合能力入口：管理飞书/Lark 产品能力（日历、邮件等）。当用户需要时使用。
---
- lark-calendar（日历）: calendar
- lark-mail（邮件）: mail
`

func TestSyncSkillsSeparateRetriesOtherDomain(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	runner := &fakeSkillsRunner{
		sources:       []string{"primary", "secondary"},
		indexes:       map[string]string{"primary": officialSkillsIndexOutput("lark-calendar", "lark-mail"), "secondary": officialSkillsIndexOutput("lark-calendar", "lark-mail")},
		installErrors: map[string]error{"primary": fmt.Errorf("digest mismatch")},
		indexErrors:   map[string]error{}, stageErrors: map[string]error{},
		globalJSON: globalSkillsJSONOutput(),
	}
	result := SyncSkills(SyncOptions{Version: "1.0.33", Layout: LayoutSeparate, Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	assertStrings(t, runner.installs, []string{"primary:lark-calendar,lark-mail", "secondary:lark-calendar,lark-mail"})
	state, _, _ := ReadState()
	if state.Layout != LayoutSeparate {
		t.Fatalf("layout = %q, want separate", state.Layout)
	}
}

func TestSyncSkillsSeparateUsesGitHubLast(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	runner := &fakeSkillsRunner{
		sources: []string{"primary", "secondary"},
		indexes: map[string]string{}, indexErrors: map[string]error{"primary": fmt.Errorf("down"), "secondary": fmt.Errorf("down")},
		installErrors: map[string]error{}, stageErrors: map[string]error{}, globalJSON: `[]`,
	}
	result := SyncSkills(SyncOptions{Version: "1.0.33", Layout: LayoutSeparate, Runner: runner, Now: time.Now})
	if result.Err != nil || result.Warning == "" {
		t.Fatalf("result = %+v, want warned success", result)
	}
	assertStrings(t, runner.installs, []string{"larksuite/cli:*"})
	if !result.OfficialUnknown || len(result.Official) != 0 {
		t.Fatalf("result = %+v, want unknown official skills", result)
	}
	state, ok, err := ReadState()
	if err != nil || !ok {
		t.Fatalf("ReadState() = (%+v, %v, %v)", state, ok, err)
	}
	if !state.OfficialSkillsUnknown || len(state.OfficialSkills) != 0 {
		t.Fatalf("state = %+v, want unknown official skills", state)
	}
}

func TestSyncSkillsSeparateRestoresAllThroughGitHubFallback(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := WriteState(SkillsState{Version: "1.0.32", Layout: LayoutSeparate, OfficialSkills: []string{"lark-calendar"}}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeSkillsRunner{
		sources:       []string{"primary"},
		indexes:       map[string]string{},
		indexErrors:   map[string]error{"primary": fmt.Errorf("down")},
		installErrors: map[string]error{},
		stageErrors:   map[string]error{},
		globalJSON:    `[]`,
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Layout: LayoutSeparate, Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	assertStrings(t, runner.installs, []string{"larksuite/cli:lark-calendar"})
	if result.Warning == "" {
		t.Fatal("warning is empty, want GitHub fallback warning")
	}
}

func TestSyncSkillsRetriesOfficialSourcesAfterUnknownFallback(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := WriteState(SkillsState{
		Version:               "1.0.33",
		Layout:                LayoutSeparate,
		OfficialSkillsUnknown: true,
	}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeSkillsRunner{
		sources:       []string{"primary"},
		indexes:       map[string]string{"primary": officialSkillsIndexOutput("lark-calendar", "lark-mail")},
		indexErrors:   map[string]error{},
		installErrors: map[string]error{},
		stageErrors:   map[string]error{},
		globalJSON:    globalSkillsJSONOutput("lark-calendar"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Layout: LayoutSeparate, Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	assertStrings(t, runner.installs, []string{"primary:lark-calendar,lark-mail"})
	if result.OfficialUnknown {
		t.Fatalf("result = %+v, want trusted official skills", result)
	}
	state, _, _ := ReadState()
	if state.OfficialSkillsUnknown {
		t.Fatalf("state = %+v, want trusted official skills", state)
	}
}

func TestSyncSkillsSuiteDoesNotUseGitHub(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	runner := &fakeSkillsRunner{
		sources: []string{"primary", "secondary"}, indexes: map[string]string{},
		indexErrors:   map[string]error{"primary": fmt.Errorf("down"), "secondary": fmt.Errorf("down")},
		installErrors: map[string]error{}, stageErrors: map[string]error{}, globalJSON: `[]`,
	}
	result := SyncSkills(SyncOptions{Version: "1.0.33", Layout: LayoutSuite, Runner: runner, Now: time.Now})
	if result.Err == nil || len(runner.installs) != 0 {
		t.Fatalf("result = %+v, installs = %v", result, runner.installs)
	}
}

func TestSyncSkillsSuiteRetriesOtherDomainAfterArchiveValidationFailure(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	runner := &fakeSkillsRunner{
		sources:       []string{"primary", "secondary"},
		indexes:       map[string]string{"primary": officialSkillsIndexOutput("lark-calendar", "lark-mail"), "secondary": officialSkillsIndexOutput("lark-calendar", "lark-mail")},
		indexErrors:   map[string]error{},
		installErrors: map[string]error{},
		stageErrors:   map[string]error{"primary": fmt.Errorf("digest mismatch")},
		globalJSON:    `[]`,
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Layout: LayoutSuite, Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	assertStrings(t, runner.stages, []string{"primary", "secondary"})
	if runner.localSuite == "" {
		t.Fatal("suite from secondary source was not installed")
	}
}

func TestSyncSkillsSuiteRetriesOtherDomainAfterChildListMismatch(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	runner := &fakeSkillsRunner{
		sources:       []string{"primary", "secondary"},
		indexes:       map[string]string{"primary": officialSkillsIndexOutput("lark-calendar", "lark-mail"), "secondary": officialSkillsIndexOutput("lark-calendar", "lark-mail")},
		indexErrors:   map[string]error{},
		installErrors: map[string]error{},
		stageErrors:   map[string]error{},
		stageChildren: map[string][]string{"primary": {"lark-calendar"}, "secondary": {"lark-calendar", "lark-mail"}},
		globalJSON:    `[]`,
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Layout: LayoutSuite, Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	assertStrings(t, runner.stages, []string{"primary", "secondary"})
}

func TestSyncSkillsSuiteFailureKeepsPreviousState(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	before := SkillsState{Version: "1.0.32", Layout: LayoutSeparate, OfficialSkills: []string{"lark-calendar", "lark-mail"}}
	if err := WriteState(before); err != nil {
		t.Fatal(err)
	}
	beforeRaw, err := os.ReadFile(statePath())
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeSkillsRunner{
		sources:       []string{"primary", "secondary"},
		indexes:       map[string]string{"primary": officialSkillsIndexOutput("lark-calendar", "lark-mail"), "secondary": officialSkillsIndexOutput("lark-calendar", "lark-mail")},
		indexErrors:   map[string]error{},
		installErrors: map[string]error{},
		stageErrors:   map[string]error{"primary": fmt.Errorf("digest mismatch"), "secondary": fmt.Errorf("archive unavailable")},
		globalJSON:    globalSkillsJSONOutput("lark-calendar", "lark-mail"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Layout: LayoutSuite, Runner: runner, Now: time.Now})
	if result.Err == nil {
		t.Fatal("expected suite sync failure")
	}
	afterRaw, err := os.ReadFile(statePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(afterRaw) != string(beforeRaw) {
		t.Fatalf("state file changed after failed sync:\ngot  %s\nwant %s", afterRaw, beforeRaw)
	}
}

func TestSyncSkillsUnreadableSuiteIsReinstalled(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := WriteState(SkillsState{
		Version:        "1.0.32",
		Layout:         LayoutSuite,
		OfficialSkills: []string{"lark-calendar", "lark-mail"},
	}); err != nil {
		t.Fatal(err)
	}
	missingSuite := filepath.Join(t.TempDir(), "missing-lark-suite")
	runner := &fakeSkillsRunner{
		sources:       []string{"primary"},
		indexes:       map[string]string{"primary": officialSkillsIndexOutput("lark-calendar", "lark-mail")},
		indexErrors:   map[string]error{},
		installErrors: map[string]error{},
		stageErrors:   map[string]error{},
		globalJSON:    fmt.Sprintf(`[{"name":"lark-suite","path":%q,"scope":"global"}]`, missingSuite),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	assertStrings(t, result.Updated, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, runner.stages, []string{"primary"})
	if runner.localSuite == "" {
		t.Fatal("damaged suite was not reinstalled")
	}
	state, _, _ := ReadState()
	if state.Layout != LayoutSuite || state.OfficialSkillsUnknown {
		t.Fatalf("state = %+v, want trusted suite state", state)
	}
}

func TestSyncSkillsEmptySuiteRestoresAllOfficial(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := WriteState(SkillsState{
		Version:        "1.0.32",
		Layout:         LayoutSuite,
		OfficialSkills: []string{"lark-calendar", "lark-mail"},
	}); err != nil {
		t.Fatal(err)
	}
	suite := t.TempDir()
	if err := os.MkdirAll(filepath.Join(suite, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeSkillsRunner{
		sources:       []string{"primary"},
		indexes:       map[string]string{"primary": officialSkillsIndexOutput("lark-calendar", "lark-mail")},
		indexErrors:   map[string]error{},
		installErrors: map[string]error{},
		stageErrors:   map[string]error{},
		globalJSON:    fmt.Sprintf(`[{"name":"lark-suite","path":%q,"scope":"global"}]`, suite),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	assertStrings(t, result.Updated, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, result.SkippedDeleted, []string{})
	assertStrings(t, runner.stages, []string{"primary"})
	if runner.localSuite == "" {
		t.Fatal("empty suite was not fully restored")
	}
}

func TestSyncSkillsMissingSuiteIsReinstalled(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := WriteState(SkillsState{
		Version:        "1.0.32",
		Layout:         LayoutSuite,
		OfficialSkills: []string{"lark-calendar", "lark-mail"},
	}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeSkillsRunner{
		sources:       []string{"primary"},
		indexes:       map[string]string{"primary": officialSkillsIndexOutput("lark-calendar", "lark-mail")},
		indexErrors:   map[string]error{},
		installErrors: map[string]error{},
		stageErrors:   map[string]error{},
		globalJSON:    `[]`,
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	assertStrings(t, result.Updated, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, runner.stages, []string{"primary"})
	if runner.localSuite == "" {
		t.Fatal("missing suite was not reinstalled")
	}
}

func TestSyncSkillsSuiteStagesCropsAndRemovesSeparate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	if err := WriteState(SkillsState{Version: "1.0.32", Layout: LayoutSeparate, OfficialSkills: []string{"lark-calendar", "lark-mail"}}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeSkillsRunner{
		sources: []string{"primary", "secondary"}, indexes: map[string]string{"primary": officialSkillsIndexOutput("lark-calendar", "lark-mail")},
		indexErrors: map[string]error{}, installErrors: map[string]error{}, stageErrors: map[string]error{},
		globalJSON: globalSkillsJSONOutput("lark-calendar"),
	}
	result := SyncSkills(SyncOptions{Version: "1.0.33", Layout: LayoutSuite, Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if runner.localSuite == "" {
		t.Fatal("cropped suite was not installed")
	}
	assertStrings(t, runner.removals[0], []string{"lark-calendar"})
	state, _, _ := ReadState()
	if state.Layout != LayoutSuite {
		t.Fatalf("layout = %q, want suite", state.Layout)
	}
	assertStrings(t, state.SkippedDeletedSkills, []string{"lark-mail"})
}

func TestSyncSkillsSwitchToSuiteRemovesPreviouslyOfficialSkillMissingFromIndex(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := WriteState(SkillsState{
		Version:        "1.0.32",
		Layout:         LayoutSeparate,
		OfficialSkills: []string{"lark-calendar", "lark-retired"},
	}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeSkillsRunner{
		sources:       []string{"primary"},
		indexes:       map[string]string{"primary": officialSkillsIndexOutput("lark-calendar")},
		indexErrors:   map[string]error{},
		installErrors: map[string]error{},
		stageErrors:   map[string]error{},
		stageChildren: map[string][]string{"primary": {"lark-calendar"}},
		globalJSON:    globalSkillsJSONOutput("lark-calendar", "lark-retired", "lark-user-owned"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Layout: LayoutSuite, Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(runner.removals) != 1 {
		t.Fatalf("removals = %v, want one removal call", runner.removals)
	}
	assertStrings(t, runner.removals[0], []string{"lark-calendar", "lark-retired"})
}

func TestSyncSkillsSwitchToSeparateRemovesSuite(t *testing.T) {
	for _, test := range []struct {
		name        string
		indexErrors map[string]error
		wantInstall string
	}{
		{name: "official source", indexErrors: map[string]error{}, wantInstall: "primary:lark-calendar,lark-mail"},
		{name: "GitHub fallback", indexErrors: map[string]error{"primary": fmt.Errorf("down")}, wantInstall: "larksuite/cli:lark-calendar,lark-mail"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			if err := WriteState(SkillsState{Version: "1.0.32", Layout: LayoutSuite, OfficialSkills: []string{"lark-calendar", "lark-mail"}}); err != nil {
				t.Fatal(err)
			}
			suite := t.TempDir()
			for _, name := range []string{"lark-calendar", "lark-mail"} {
				if err := os.MkdirAll(filepath.Join(suite, "references", name), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			runner := &fakeSkillsRunner{
				sources:       []string{"primary"},
				indexes:       map[string]string{"primary": officialSkillsIndexOutput("lark-calendar", "lark-mail")},
				indexErrors:   test.indexErrors,
				installErrors: map[string]error{},
				stageErrors:   map[string]error{},
				globalJSON:    fmt.Sprintf(`[{"name":"lark-suite","path":%q,"scope":"global"}]`, suite),
			}

			result := SyncSkills(SyncOptions{Version: "1.0.33", Layout: LayoutSeparate, Runner: runner, Now: time.Now})
			if result.Err != nil {
				t.Fatal(result.Err)
			}
			assertStrings(t, runner.installs, []string{test.wantInstall})
			if len(runner.removals) != 1 {
				t.Fatalf("removals = %v, want one removal call", runner.removals)
			}
			assertStrings(t, runner.removals[0], []string{"lark-suite"})
		})
	}
}

func TestPrepareSuiteCropsRoutesKeywordsAndReferences(t *testing.T) {
	suite := t.TempDir()
	for _, name := range []string{"lark-calendar", "lark-mail"} {
		if err := os.MkdirAll(filepath.Join(suite, "references", name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(suite, "SKILL.md"), []byte(suiteFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := prepareSuite(suite, []string{"lark-calendar", "lark-mail"}, []string{"lark-calendar"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(suite, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if strings.Contains(content, "lark-mail") || strings.Contains(content, "日历、邮件等") || !strings.Contains(content, "（日历等）") || !strings.Contains(content, "- lark-calendar（日历）: calendar") {
		t.Fatalf("unexpected cropped SKILL.md:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(suite, "references", "lark-mail")); !os.IsNotExist(err) {
		t.Fatalf("removed reference still exists: %v", err)
	}
}

func TestCropSuiteRoutesRemovesBoundaryLinesWithoutChangingNeighbors(t *testing.T) {
	routes := []string{
		"- lark-approval（审批）: approval",
		"- lark-calendar（日历）: calendar",
		"- lark-mail（邮件）: mail",
	}
	prefix := "description: 飞书/Lark 聚合能力入口：管理飞书/Lark 产品能力（审批、日历、邮件等）。\nbefore\n"
	for _, test := range []struct {
		name, removed, keywords, suffix string
		target                          []string
	}{
		{name: "first", removed: "lark-approval", keywords: "日历、邮件", target: []string{"lark-calendar", "lark-mail"}, suffix: "\nafter\n"},
		{name: "middle", removed: "lark-calendar", keywords: "审批、邮件", target: []string{"lark-approval", "lark-mail"}, suffix: "\nafter\n"},
		{name: "last", removed: "lark-mail", keywords: "审批、日历", target: []string{"lark-approval", "lark-calendar"}, suffix: "\nafter\n"},
		{name: "last at EOF", removed: "lark-mail", keywords: "审批、日历", target: []string{"lark-approval", "lark-calendar"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := prefix + strings.Join(routes, "\n") + test.suffix
			got, err := cropSuiteRoutes(content, []string{test.removed}, test.target)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Replace(content, "审批、日历、邮件", test.keywords, 1)
			line := routes[0]
			for _, candidate := range routes {
				if strings.Contains(candidate, test.removed) {
					line = candidate
					break
				}
			}
			want = strings.Replace(want, line+"\n", "", 1)
			want = strings.TrimSuffix(want, line)
			if got != want {
				t.Fatalf("cropped content changed surrounding lines\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestSyncSkillsNilRunnerFails(t *testing.T) {
	result := SyncSkills(SyncOptions{Version: "1.0.33", Now: time.Now})
	if result.Err == nil {
		t.Fatal("expected nil runner failure")
	}
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
