// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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

func TestParseOfficialSkillsListAcceptsNonLarkOfficialNames(t *testing.T) {
	input := `Available Skills
│    lark-calendar
│    official-shared
│    bad/name
`
	got := ParseSkillsList(input)
	want := []string{"lark-calendar", "official-shared"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSkillsList() (Available Skills) = %#v, want %#v", got, want)
	}
}

func TestParseFlatSkillsTrimsDeduplicatesAndSorts(t *testing.T) {
	got := ParseFlatSkills(" lark-doc, lark-im,,lark-doc, lark-base ")
	want := []string{"lark-base", "lark-doc", "lark-im"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseFlatSkills() = %#v, want %#v", got, want)
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

func TestParseGlobalSkillsJSON(t *testing.T) {
	input := `[
  {"name":"lark-calendar","path":"/Users/example/.agents/skills/lark-calendar","scope":"global","agents":["Codex"]},
  {"name":"lark-mail@1.2.3","path":"/Users/example/.agents/skills/lark-mail","scope":"global","agents":["Codex"]},
  {"name":"lark-calendar","path":"/Users/example/.agents/skills/lark-calendar","scope":"global","agents":["Codex"]},
  {"name":"  lark-base  ","path":"/Users/example/.agents/skills/lark-base","scope":"global","agents":["Codex"]},
  {"name":""},
  {"name":"   "},
  {"name":"bad skill"}
]`
	got := ParseGlobalSkillsJSON(input)
	want := []string{"lark-base", "lark-calendar", "lark-mail@1.2.3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseGlobalSkillsJSON() = %#v, want %#v", got, want)
	}
}

func TestParseGlobalSkillsJSONInvalidOrUnsupported(t *testing.T) {
	for _, input := range []string{
		`not json`,
		`{"name":"lark-calendar"}`,
		`[]`,
	} {
		if got := ParseGlobalSkillsJSON(input); len(got) != 0 {
			t.Fatalf("ParseGlobalSkillsJSON(%q) = %#v, want empty", input, got)
		}
	}
}

func TestParseOfficialSkillsIndexJSON(t *testing.T) {
	input := `{
  "skills": [
    {"name":"lark-calendar","description":"Calendar","files":["SKILL.md"]},
    {"name":"lark-mail","description":"Mail","files":["SKILL.md","references/lark-mail-search.md"]},
    {"name":"  lark-base  ","description":"Base","files":[]},
    {"name":"lark-calendar","description":"duplicate","files":["SKILL.md"]},
    {"name":"custom-skill","description":"not official","files":["SKILL.md"]},
    {"name":"bad skill","description":"invalid","files":["SKILL.md"]},
    {"name":"","description":"empty","files":["SKILL.md"]}
  ]
}`
	got, err := ParseOfficialSkillsIndexJSON(input)
	if err != nil {
		t.Fatalf("ParseOfficialSkillsIndexJSON() err = %v, want nil", err)
	}
	want := []string{"custom-skill", "lark-base", "lark-calendar", "lark-mail"}
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
		`{"skills":[{"name":"bad skill"}]}`,
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
	officialIndexOut string
	officialOut      string
	globalJSONOut    string
	globalOut        string
	officialIndexErr error
	officialErr      error
	globalJSONErr    error
	globalErr        error
	installErr       error
	installAllErr    error
	installSuiteErr  error
	installed        [][]string
	installedAll     int
	installedSuite   int
	listedIndex      int
	listedOfficial   int
	listedGlobalJSON int
	listedGlobalText int
}

func officialSkillsOutput(names ...string) string {
	var b strings.Builder
	b.WriteString("Available Skills\n")
	for _, name := range names {
		b.WriteString("│    ")
		b.WriteString(name)
		b.WriteString("\n")
	}
	return b.String()
}

func officialSkillsIndexOutput(names ...string) string {
	var b strings.Builder
	b.WriteString(`{"skills":[`)
	for i, name := range names {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"name":%q,"description":"test skill","files":["SKILL.md"]}`, name)
	}
	b.WriteString(`]}`)
	return b.String()
}

func globalSkillsOutput(names ...string) string {
	var b strings.Builder
	b.WriteString("Global Skills\n\n")
	for _, name := range names {
		b.WriteString(name)
		b.WriteString(" ~/.agents/skills/")
		b.WriteString(name)
		b.WriteString("\n  Agents: Claude Code\n")
	}
	return b.String()
}

func globalSkillsJSONOutput(names ...string) string {
	var b strings.Builder
	b.WriteString("[")
	for i, name := range names {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"name":%q,"path":"/Users/example/.agents/skills/%s","scope":"global","agents":["Codex"]}`, name, name)
	}
	b.WriteString("]")
	return b.String()
}

func (f *fakeSkillsRunner) ListOfficialSkillsIndex() *selfupdate.NpmResult {
	f.listedIndex++
	r := &selfupdate.NpmResult{}
	r.Stdout.WriteString(f.officialIndexOut)
	r.Err = f.officialIndexErr
	return r
}

func (f *fakeSkillsRunner) ListOfficialSkills() *selfupdate.NpmResult {
	f.listedOfficial++
	r := &selfupdate.NpmResult{}
	r.Stdout.WriteString(f.officialOut)
	r.Err = f.officialErr
	return r
}

func (f *fakeSkillsRunner) ListGlobalSkillsJSON() *selfupdate.NpmResult {
	f.listedGlobalJSON++
	r := &selfupdate.NpmResult{}
	r.Stdout.WriteString(f.globalJSONOut)
	r.Err = f.globalJSONErr
	return r
}

func (f *fakeSkillsRunner) ListGlobalSkills() *selfupdate.NpmResult {
	f.listedGlobalText++
	r := &selfupdate.NpmResult{}
	r.Stdout.WriteString(f.globalOut)
	r.Err = f.globalErr
	return r
}

func (f *fakeSkillsRunner) InstallSkill(nameList []string) *selfupdate.NpmResult {
	f.installed = append(f.installed, nameList)
	r := &selfupdate.NpmResult{}
	r.Err = f.installErr
	return r
}

func (f *fakeSkillsRunner) InstallAllSkills() *selfupdate.NpmResult {
	f.installedAll++
	r := &selfupdate.NpmResult{}
	r.Err = f.installAllErr
	return r
}

func (f *fakeSkillsRunner) InstallSuiteSkill() *selfupdate.NpmResult {
	f.installedSuite++
	r := &selfupdate.NpmResult{}
	r.Err = f.installSuiteErr
	return r
}

func TestSyncSkills_WritesStateAndDoesNotWriteStamp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	if err := WriteState(SkillsState{
		Version:        "1.0.30",
		OfficialSkills: []string{"lark-calendar", "lark-mail"},
		UpdatedAt:      "2026-05-18T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-mail", "lark-new"),
		officialOut:      officialSkillsOutput("lark-calendar", "lark-mail", "lark-new"),
		globalJSONOut:    globalSkillsJSONOutput("lark-calendar", "lark-custom"),
		globalOut:        globalSkillsOutput("lark-mail"),
	}
	result := SyncSkills(SyncOptions{
		Version: "1.0.33",
		Runner:  runner,
		Now:     func() time.Time { return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC) },
	})

	if result.Err != nil {
		t.Fatalf("SyncSkills() err = %v, want nil", result.Err)
	}
	assertStrings(t, runner.installed[0], []string{"lark-calendar", "lark-new"})
	if runner.listedGlobalJSON != 1 {
		t.Fatalf("listedGlobalJSON = %d, want 1", runner.listedGlobalJSON)
	}
	if runner.listedGlobalText != 0 {
		t.Fatalf("listedGlobalText = %d, want 0 when JSON list succeeds", runner.listedGlobalText)
	}

	state, readable, err := ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	assertStrings(t, state.OfficialSkills, []string{"lark-calendar", "lark-mail", "lark-new"})
	assertStrings(t, state.UpdatedSkills, []string{"lark-calendar", "lark-new"})
	assertStrings(t, state.AddedOfficialSkills, []string{"lark-new"})
	assertStrings(t, state.SkippedDeletedSkills, []string{"lark-mail"})
	if _, err := os.Stat(filepath.Join(dir, "skills.stamp")); !os.IsNotExist(err) {
		t.Fatalf("skills.stamp exists or stat failed with unexpected err: %v", err)
	}
}

func TestSyncSkills_OfficialIndexSuccessSkipsOfficialListCommand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-mail", "lark-new"),
		officialOut:      officialSkillsOutput("lark-should-not-be-used"),
		globalJSONOut:    globalSkillsJSONOutput("lark-calendar"),
		globalOut:        globalSkillsOutput("lark-mail"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatalf("SyncSkills() err = %v, want nil", result.Err)
	}
	assertStrings(t, result.Official, []string{"lark-calendar", "lark-mail", "lark-new"})
	assertStrings(t, runner.installed[0], []string{"lark-calendar", "lark-mail", "lark-new"})
	if runner.listedIndex != 1 {
		t.Fatalf("listedIndex = %d, want 1", runner.listedIndex)
	}
	if runner.listedOfficial != 0 {
		t.Fatalf("listedOfficial = %d, want 0 when index succeeds", runner.listedOfficial)
	}
}

func TestSyncSkills_OfficialIndexFailureFallsBackToOfficialList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexErr: fmt.Errorf("index unavailable"),
		officialOut:      officialSkillsOutput("lark-calendar", "lark-mail"),
		globalJSONOut:    globalSkillsJSONOutput("lark-calendar"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatalf("SyncSkills() err = %v, want nil", result.Err)
	}
	assertStrings(t, result.Official, []string{"lark-calendar", "lark-mail"})
	if runner.listedIndex != 1 || runner.listedOfficial != 1 {
		t.Fatalf("listed index/official = %d/%d, want 1/1", runner.listedIndex, runner.listedOfficial)
	}
	if runner.installedAll != 0 {
		t.Fatalf("installedAll = %d, want 0", runner.installedAll)
	}
}

func TestSyncSkills_OfficialIndexEmptyFallsBackToOfficialList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexOut: `{"skills":[]}`,
		officialOut:      officialSkillsOutput("lark-calendar", "lark-mail"),
		globalJSONOut:    globalSkillsJSONOutput("lark-calendar"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatalf("SyncSkills() err = %v, want nil", result.Err)
	}
	assertStrings(t, result.Official, []string{"lark-calendar", "lark-mail"})
	if runner.listedIndex != 1 || runner.listedOfficial != 1 {
		t.Fatalf("listed index/official = %d/%d, want 1/1", runner.listedIndex, runner.listedOfficial)
	}
}

func TestSyncSkills_OfficialDiscoveryFailuresFallBackToFullInstallWithReasons(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexErr: fmt.Errorf("index unavailable"),
		officialErr:      fmt.Errorf("list failed"),
		installAllErr:    nil,
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_synced" {
		t.Fatalf("SyncSkills() action = %q, want fallback_synced", result.Action)
	}
	if runner.installedAll != 1 {
		t.Fatalf("installedAll = %d, want 1", runner.installedAll)
	}
	if !strings.Contains(result.Detail, "official skills index failed") || !strings.Contains(result.Detail, "official skills list failed") {
		t.Fatalf("SyncSkills() detail = %q, want both discovery failure reasons", result.Detail)
	}
}

func TestSyncSkills_OfficialDiscoveryEmptyFallsBackToFullInstallWithReasons(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexOut: `{"skills":[]}`,
		installAllErr:    nil,
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_synced" {
		t.Fatalf("SyncSkills() action = %q, want fallback_synced", result.Action)
	}
	if runner.installedAll != 1 {
		t.Fatalf("installedAll = %d, want 1", runner.installedAll)
	}
	if !strings.Contains(result.Detail, "official skills index contains no skills") || !strings.Contains(result.Detail, "official skills list returned no skills") {
		t.Fatalf("SyncSkills() detail = %q, want both empty discovery reasons", result.Detail)
	}
}

func TestSyncSkills_HybridOfficialDiscoveryFailureDoesNotFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexErr: fmt.Errorf("index unavailable"),
		officialErr:      fmt.Errorf("list unavailable"),
	}

	result := SyncSkills(SyncOptions{
		Version: "1.0.33",
		Layout:  LayoutHybrid,
		Runner:  runner,
		Now:     time.Now,
	})
	if result.Action != "failed" {
		t.Fatalf("SyncSkills() action = %q, want failed", result.Action)
	}
	if result.Layout != LayoutHybrid {
		t.Fatalf("SyncSkills() layout = %q, want %q", result.Layout, LayoutHybrid)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "failed to discover official skills for hybrid layout") {
		t.Fatalf("SyncSkills() err = %v, want hybrid discovery failure", result.Err)
	}
	if runner.installedAll != 0 {
		t.Fatalf("installedAll = %d, want 0", runner.installedAll)
	}
}

func TestSyncSkills_ListOfficialFailureFallsBackToFullInstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexErr: fmt.Errorf("index unavailable"),
		officialErr:      fmt.Errorf("list failed"),
		installAllErr:    nil,
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_synced" {
		t.Fatalf("SyncSkills() action = %q, want fallback_synced", result.Action)
	}
	if runner.installedAll != 1 {
		t.Fatalf("installedAll = %d, want 1", runner.installedAll)
	}
	if len(runner.installed) != 0 {
		t.Fatalf("installed = %#v, want no incremental installs", runner.installed)
	}

	state, readable, err := ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	if state.Version != "1.0.33" {
		t.Fatalf("state.Version = %q, want %q", state.Version, "1.0.33")
	}
	assertStrings(t, state.OfficialSkills, []string{})
}

func TestSyncSkills_ListOfficialFailureAndFullInstallFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexErr: fmt.Errorf("index unavailable"),
		officialErr:      fmt.Errorf("list failed"),
		installAllErr:    fmt.Errorf("full install failed"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_failed" {
		t.Fatalf("SyncSkills() action = %q, want fallback_failed", result.Action)
	}
	if result.Err == nil {
		t.Fatalf("SyncSkills() err = nil, want error")
	}
	if !strings.Contains(result.Err.Error(), "full skills install failed") {
		t.Fatalf("SyncSkills() err = %v, want full install failure", result.Err)
	}
}

func TestSyncSkills_GlobalJSONFailureFallsBackToTextList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-mail"),
		officialOut:      officialSkillsOutput("lark-calendar", "lark-mail"),
		globalJSONErr:    fmt.Errorf("json list failed"),
		globalOut:        globalSkillsOutput("lark-calendar"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatalf("SyncSkills() err = %v, want nil", result.Err)
	}
	if result.Action != "synced" {
		t.Fatalf("SyncSkills() action = %q, want synced", result.Action)
	}
	assertStrings(t, result.Updated, []string{"lark-calendar", "lark-mail"})
	if runner.listedGlobalJSON != 1 || runner.listedGlobalText != 1 {
		t.Fatalf("listed JSON/text = %d/%d, want 1/1", runner.listedGlobalJSON, runner.listedGlobalText)
	}
	if runner.installedAll != 0 {
		t.Fatalf("installedAll = %d, want 0", runner.installedAll)
	}
}

func TestSyncSkills_LocalListsFailureFallsBackToFullInstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-mail"),
		officialOut:      officialSkillsOutput("lark-calendar", "lark-mail"),
		globalJSONErr:    fmt.Errorf("json list failed with /Users/example/.agents/skills/lark-calendar agents Codex"),
		globalErr:        fmt.Errorf("text list failed with /Users/example/.agents/skills/lark-mail agents Codex"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_synced" {
		t.Fatalf("SyncSkills() action = %q, want fallback_synced", result.Action)
	}
	if len(runner.installed) != 0 {
		t.Fatalf("installed = %#v, want no incremental installs", runner.installed)
	}
	if runner.installedAll != 1 {
		t.Fatalf("installedAll = %d, want 1", runner.installedAll)
	}
	if strings.Contains(result.Detail, "/Users/example") || strings.Contains(result.Detail, "agents") {
		t.Fatalf("SyncSkills() detail leaks local command output: %q", result.Detail)
	}
}

func TestSyncSkills_EmptyGlobalJSONInstallsAllOfficialIncrementally(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-mail"),
		officialOut:      officialSkillsOutput("lark-calendar", "lark-mail"),
		globalJSONOut:    `[]`,
		globalOut:        "Some unrecognized output format\n",
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "synced" {
		t.Fatalf("SyncSkills() action = %q, want synced", result.Action)
	}
	if len(runner.installed) != 1 {
		t.Fatalf("installed = %#v, want one incremental install", runner.installed)
	}
	assertStrings(t, runner.installed[0], []string{"lark-calendar", "lark-mail"})
	if runner.installedAll != 0 {
		t.Fatalf("installedAll = %d, want 0", runner.installedAll)
	}
}

func TestSyncSkills_EmptyToUpdateFallsBackToFullInstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	if err := WriteState(SkillsState{
		Version:        "1.0.30",
		OfficialSkills: []string{"lark-calendar", "lark-mail"},
		UpdatedAt:      "2026-05-18T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-mail"),
		officialOut:      officialSkillsOutput("lark-calendar", "lark-mail"),
		globalOut:        globalSkillsOutput(),
		installAllErr:    nil,
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_synced" {
		t.Fatalf("SyncSkills() action = %q, want fallback_synced", result.Action)
	}
	if len(runner.installed) != 0 {
		t.Fatalf("installed = %#v, want no incremental installs", runner.installed)
	}
	if runner.installedAll != 1 {
		t.Fatalf("installedAll = %d, want 1 (fallback triggered)", runner.installedAll)
	}
	assertStrings(t, result.Official, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, result.Updated, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, result.Added, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, result.SkippedDeleted, []string{})
}

func TestSyncSkills_InstallFailureFallsBackToFullInstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-mail"),
		officialOut:      officialSkillsOutput("lark-calendar", "lark-mail"),
		globalJSONOut:    globalSkillsJSONOutput("lark-calendar", "lark-mail"),
		globalOut:        globalSkillsOutput("lark-calendar", "lark-mail"),
		installErr:       fmt.Errorf("incremental boom"),
		installAllErr:    nil,
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_synced" {
		t.Fatalf("SyncSkills() action = %q, want fallback_synced", result.Action)
	}
	if len(runner.installed) != 1 {
		t.Fatalf("installed = %d calls, want 1", len(runner.installed))
	}
	if runner.installedAll != 1 {
		t.Fatalf("installedAll = %d, want 1 (fallback triggered)", runner.installedAll)
	}

	state, readable, err := ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	if state.Version != "1.0.33" {
		t.Fatalf("state.Version = %q, want %q", state.Version, "1.0.33")
	}
	assertStrings(t, state.OfficialSkills, []string{"lark-calendar", "lark-mail"})
}

func TestSyncSkills_InstallFailureAndFullInstallFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-mail"),
		officialOut:      officialSkillsOutput("lark-calendar", "lark-mail"),
		globalJSONOut:    globalSkillsJSONOutput("lark-calendar", "lark-mail"),
		globalOut:        globalSkillsOutput("lark-calendar", "lark-mail"),
		installErr:       fmt.Errorf("incremental boom"),
		installAllErr:    fmt.Errorf("full install boom"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_failed" {
		t.Fatalf("SyncSkills() action = %q, want fallback_failed", result.Action)
	}
	if result.Err == nil {
		t.Fatalf("SyncSkills() err = nil, want error")
	}
	if !strings.Contains(result.Detail, "incremental boom") {
		t.Fatalf("SyncSkills() detail = %q, want incremental error text", result.Detail)
	}
	if !strings.Contains(result.Err.Error(), "full skills install failed") {
		t.Fatalf("SyncSkills() err = %v, want full install failure", result.Err)
	}
}

func TestSyncSkills_NilRunnerFails(t *testing.T) {
	result := SyncSkills(SyncOptions{Version: "1.0.33", Now: time.Now})
	if result.Err == nil || !strings.Contains(result.Err.Error(), "skills runner is nil") {
		t.Fatalf("SyncSkills() err = %v, want nil runner failure", result.Err)
	}
}

func TestSyncSkills_HybridAssemblesSuiteAndMovesCollectedSkills(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	paths := map[string]string{}
	for _, name := range []string{"lark-calendar", "lark-doc", "lark-shared", "lark-suite"} {
		paths[name] = filepath.Join(dir, name)
		writeTestSkill(t, paths[name], name)
	}

	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-doc", "lark-shared"),
		globalJSONOut:    globalSkillsJSONFromPaths(paths),
	}

	result := SyncSkills(SyncOptions{
		Version:    "1.0.33",
		Layout:     LayoutHybrid,
		FlatSkills: []string{"lark-calendar"},
		Runner:     runner,
		Now:        time.Now,
	})

	if result.Err != nil {
		t.Fatalf("SyncSkills() err = %v, want nil", result.Err)
	}
	if runner.installedSuite != 1 {
		t.Fatalf("installedSuite = %d, want 1", runner.installedSuite)
	}
	assertStrings(t, result.Flat, []string{"lark-calendar"})
	assertStrings(t, result.Collected, []string{"lark-shared", "lark-doc"})
	if _, err := os.Stat(filepath.Join(paths["lark-suite"], "references", "subskills", "lark-doc", "SKILL.md")); err != nil {
		t.Fatalf("suite lark-doc missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths["lark-suite"], "references", "subskills", "lark-shared", "SKILL.md")); err != nil {
		t.Fatalf("suite lark-shared missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths["lark-calendar"], "SKILL.md")); err != nil {
		t.Fatalf("flat lark-calendar missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths["lark-doc"], "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("collected lark-doc still exists at top level or unexpected err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths["lark-shared"], "SKILL.md")); err != nil {
		t.Fatalf("lark-shared should stay top-level when flat set is non-empty: %v", err)
	}

	state, readable, err := ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	if state.Layout != LayoutHybrid {
		t.Fatalf("state.Layout = %q, want %q", state.Layout, LayoutHybrid)
	}
	assertStrings(t, state.FlatSkills, []string{"lark-calendar"})
}

func TestSyncSkills_HybridCollectsNewOfficialSkillOnNextUpdate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	paths := map[string]string{}
	for _, name := range []string{"lark-calendar", "lark-doc", "lark-shared", "lark-suite"} {
		paths[name] = filepath.Join(dir, name)
		writeTestSkill(t, paths[name], name)
	}

	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-doc", "lark-shared"),
		globalJSONOut:    globalSkillsJSONFromPaths(paths),
	}
	first := SyncSkills(SyncOptions{
		Version:    "1.0.33",
		Layout:     LayoutHybrid,
		FlatSkills: []string{"lark-calendar"},
		Runner:     runner,
		Now:        time.Now,
	})
	if first.Err != nil {
		t.Fatalf("first SyncSkills() err = %v, want nil", first.Err)
	}

	paths["lark-new"] = filepath.Join(dir, "lark-new")
	for _, name := range []string{"lark-doc", "lark-new", "lark-shared", "lark-suite"} {
		writeTestSkill(t, paths[name], name)
	}
	runner.officialIndexOut = officialSkillsIndexOutput("lark-calendar", "lark-doc", "lark-new", "lark-shared")
	runner.globalJSONOut = globalSkillsJSONFromPaths(paths)

	second := SyncSkills(SyncOptions{
		Version:    "1.0.34",
		Layout:     LayoutHybrid,
		FlatSkills: []string{"lark-calendar"},
		Runner:     runner,
		Now:        time.Now,
	})
	if second.Err != nil {
		t.Fatalf("second SyncSkills() err = %v, want nil", second.Err)
	}
	assertStrings(t, second.Added, []string{"lark-new"})
	assertStrings(t, second.Flat, []string{"lark-calendar"})
	assertStrings(t, second.Collected, []string{"lark-shared", "lark-doc", "lark-new"})

	if _, err := os.Stat(filepath.Join(paths["lark-suite"], "references", "subskills", "lark-new", "SKILL.md")); err != nil {
		t.Fatalf("suite lark-new missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths["lark-new"], "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("new official skill should be collected, top-level path err: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(paths["lark-suite"], "SKILL.md"))
	if err != nil {
		t.Fatalf("read suite SKILL.md: %v", err)
	}
	if !strings.Contains(string(data), "- lark-new: lark-new description") {
		t.Fatalf("suite SKILL.md missing lark-new route:\n%s", data)
	}

	state, readable, err := ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	assertStrings(t, state.OfficialSkills, []string{"lark-calendar", "lark-doc", "lark-new", "lark-shared"})
	assertStrings(t, state.AddedOfficialSkills, []string{"lark-new"})
	assertStrings(t, state.FlatSkills, []string{"lark-calendar"})
}

func TestSyncSkills_HybridWithNoFlatSkillsDoesNotKeepSharedTopLevel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	paths := map[string]string{}
	for _, name := range []string{"lark-shared", "lark-suite"} {
		paths[name] = filepath.Join(dir, name)
		writeTestSkill(t, paths[name], name)
	}

	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-shared"),
		globalJSONOut:    globalSkillsJSONFromPaths(paths),
	}

	result := SyncSkills(SyncOptions{
		Version: "1.0.33",
		Layout:  LayoutHybrid,
		Runner:  runner,
		Now:     time.Now,
	})

	if result.Err != nil {
		t.Fatalf("SyncSkills() err = %v, want nil", result.Err)
	}
	if _, err := os.Stat(filepath.Join(paths["lark-shared"], "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("lark-shared should not stay top-level when flat set is empty; err: %v", err)
	}
	if result.Flat == nil || len(result.Flat) != 0 {
		t.Fatalf("result.Flat = %#v, want empty slice", result.Flat)
	}
}

func TestSyncSkills_HybridFiltersNonFlatOfficialSkills(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	paths := map[string]string{}
	for _, name := range []string{"lark-calendar", "lark-shared", "lark-suite"} {
		paths[name] = filepath.Join(dir, name)
		writeTestSkill(t, paths[name], name)
	}

	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-shared"),
		globalJSONOut:    globalSkillsJSONFromPaths(paths),
	}

	result := SyncSkills(SyncOptions{
		Version:    "1.0.33",
		Layout:     LayoutHybrid,
		FlatSkills: []string{"lark-calendar", "lark-missing", "lark-shared"},
		Runner:     runner,
		Now:        time.Now,
	})

	if result.Err != nil {
		t.Fatalf("SyncSkills() err = %v, want nil", result.Err)
	}
	assertStrings(t, result.Flat, []string{"lark-calendar"})

	state, readable, err := ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	assertStrings(t, state.FlatSkills, []string{"lark-calendar"})
}

func TestSyncSkills_ParseEmptyWithNonEmptyStdoutFallsBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexErr: fmt.Errorf("index unavailable"),
		officialOut:      "Some unrecognized output format\n",
		installAllErr:    nil,
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_synced" {
		t.Fatalf("SyncSkills() action = %q, want fallback_synced", result.Action)
	}
	if runner.installedAll != 1 {
		t.Fatalf("installedAll = %d, want 1", runner.installedAll)
	}
}

func TestSyncSkills_ParseEmptyWithNonEmptyStdoutAndFullInstallFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexErr: fmt.Errorf("index unavailable"),
		officialOut:      "Some unrecognized output format\n",
		installAllErr:    fmt.Errorf("full install failed"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_failed" {
		t.Fatalf("SyncSkills() action = %q, want fallback_failed", result.Action)
	}
	if result.Err == nil {
		t.Fatalf("SyncSkills() err = nil, want error")
	}
}

func TestNormalizeSuiteTemplateTextRewritesLegacyFlatSkillWording(t *testing.T) {
	input := "`lark-shared` 是共享基础能力，不作为 `--collected-skills` 的可选项。为了保证 suite 内子能力可用，hybrid 布局会同时保留顶层 `lark-shared`，并在 `lark-suite/references/subskills/lark-shared/SKILL.md` 中维护一份副本。"
	got := normalizeSuiteTemplateText(input)
	if strings.Contains(got, "--collected-skills") {
		t.Fatalf("normalized text still contains legacy flag: %s", got)
	}
	if !strings.Contains(got, "--flat-skills") {
		t.Fatalf("normalized text missing --flat-skills: %s", got)
	}
	if !strings.Contains(got, "只有 hybrid 布局存在平铺 skill 时，顶层才会额外保留一份") {
		t.Fatalf("normalized text missing current lark-shared rule: %s", got)
	}
}

func TestSkillDescriptionSupportsFoldedYAMLScalar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := `---
name: lark-whiteboard
description: >
  飞书画板：查询和编辑飞书云文档中的画板。
  当用户需要查看画板内容、导出画板图片、编辑画板时使用此 skill。
metadata:
  requires:
    bins: ["lark-cli"]
---
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := skillDescription(path)
	want := "飞书画板：查询和编辑飞书云文档中的画板。 当用户需要查看画板内容、导出画板图片、编辑画板时使用此 skill。"
	if got != want {
		t.Fatalf("skillDescription() = %q, want %q", got, want)
	}
}

func TestAssembleSuiteLayoutSeparateIsNoop(t *testing.T) {
	if err := assembleSuiteLayout(LayoutSeparate, []string{"lark-doc"}, false, nil); err != nil {
		t.Fatalf("assembleSuiteLayout(separate) err = %v, want nil", err)
	}
}

func TestAssembleSuiteLayoutMissingSuiteReturnsError(t *testing.T) {
	err := assembleSuiteLayout(LayoutHybrid, []string{"lark-doc"}, false, []GlobalSkillInfo{
		{Name: "lark-doc", Path: filepath.Join(t.TempDir(), "lark-doc")},
	})
	if err == nil || !strings.Contains(err.Error(), "lark-suite") {
		t.Fatalf("assembleSuiteLayout() err = %v, want missing lark-suite error", err)
	}
}

func TestCopyDirCopiesNestedFiles(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir() err = %v, want nil", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "nested", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("copied file = %q, want hello", string(got))
	}
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func writeTestSkill(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("---\nname: %s\ndescription: %s description\n---\n", name, name)
	if name == suiteSkillName {
		content = "---\nname: lark-suite\ndescription: Lark suite\n---\n<!-- LARK_SUITE_ROUTES -->\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func globalSkillsJSONFromPaths(paths map[string]string) string {
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("[")
	for i, name := range names {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"name":%q,"path":%q,"scope":"global","agents":["Codex"]}`, name, paths[name])
	}
	b.WriteString("]")
	return b.String()
}

func TestSyncSkills_FallbackWithUnknownOfficialWritesMinimalState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexErr: fmt.Errorf("index unavailable"),
		officialOut:      "Some unrecognized output format\n",
		installAllErr:    nil,
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_synced" {
		t.Fatalf("SyncSkills() action = %q, want fallback_synced", result.Action)
	}

	state, readable, err := ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	if state.Version != "1.0.33" {
		t.Fatalf("state.Version = %q, want %q", state.Version, "1.0.33")
	}
	assertStrings(t, state.OfficialSkills, []string{})
	assertStrings(t, state.UpdatedSkills, []string{})
	assertStrings(t, state.AddedOfficialSkills, []string{})
}

func TestSyncSkills_FallbackWithKnownOfficialWritesFullState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-mail"),
		officialOut:      officialSkillsOutput("lark-calendar", "lark-mail"),
		globalJSONOut:    globalSkillsJSONOutput("lark-calendar", "lark-mail"),
		globalOut:        globalSkillsOutput("lark-calendar", "lark-mail"),
		installErr:       fmt.Errorf("incremental boom"),
		installAllErr:    nil,
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_synced" {
		t.Fatalf("SyncSkills() action = %q, want fallback_synced", result.Action)
	}

	state, readable, err := ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	assertStrings(t, state.OfficialSkills, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, state.UpdatedSkills, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, state.AddedOfficialSkills, []string{"lark-calendar", "lark-mail"})
}

func TestSyncSkills_FallbackResultContainsMetadata(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-mail"),
		officialOut:      officialSkillsOutput("lark-calendar", "lark-mail"),
		globalJSONOut:    globalSkillsJSONOutput("lark-calendar", "lark-mail"),
		globalOut:        globalSkillsOutput("lark-calendar", "lark-mail"),
		installErr:       fmt.Errorf("incremental boom"),
		installAllErr:    nil,
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_synced" {
		t.Fatalf("SyncSkills() action = %q, want fallback_synced", result.Action)
	}
	assertStrings(t, result.Official, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, result.Updated, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, result.Added, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, result.SkippedDeleted, []string{})
	if !strings.Contains(result.Detail, "incremental boom") {
		t.Fatalf("SyncSkills() detail = %q, want incremental error text", result.Detail)
	}
}

func TestSyncSkills_FallbackBreaksDegradationLoop(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialIndexErr: fmt.Errorf("index unavailable"),
		officialErr:      fmt.Errorf("list failed"),
		installAllErr:    nil,
	}

	result1 := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result1.Action != "fallback_synced" {
		t.Fatalf("first sync: action = %q, want fallback_synced", result1.Action)
	}

	state, readable, err := ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() after first sync = (_, %v, %v), want readable", readable, err)
	}
	if state.Version != "1.0.33" {
		t.Fatalf("state.Version = %q, want %q", state.Version, "1.0.33")
	}

	runner2 := &fakeSkillsRunner{
		officialIndexOut: officialSkillsIndexOutput("lark-calendar", "lark-mail"),
		officialOut:      officialSkillsOutput("lark-calendar", "lark-mail"),
		globalJSONOut:    globalSkillsJSONOutput("lark-calendar", "lark-mail"),
		globalOut:        globalSkillsOutput("lark-calendar", "lark-mail"),
	}
	result2 := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner2, Now: time.Now})
	if result2.Action != "synced" {
		t.Fatalf("second sync: action = %q, want synced (no fallback loop)", result2.Action)
	}
	if runner2.installedAll != 0 {
		t.Fatalf("second sync: installedAll = %d, want 0 (incremental, not fallback)", runner2.installedAll)
	}
}
