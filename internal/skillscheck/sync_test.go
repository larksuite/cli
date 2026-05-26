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
	officialOut   string
	globalOut     string
	officialErr   error
	globalErr     error
	installErr    error
	installAllErr error
	installed     [][]string
	installedAll  int
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

func (f *fakeSkillsRunner) ListOfficialSkills() *selfupdate.NpmResult {
	r := &selfupdate.NpmResult{}
	r.Stdout.WriteString(f.officialOut)
	r.Err = f.officialErr
	return r
}

func (f *fakeSkillsRunner) ListGlobalSkills() *selfupdate.NpmResult {
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
		officialOut: officialSkillsOutput("lark-calendar", "lark-mail", "lark-new"),
		globalOut:   globalSkillsOutput("lark-calendar", "lark-custom"),
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

func TestSyncSkills_ListOfficialFailureFallsBackToFullInstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialErr:   fmt.Errorf("list failed"),
		installAllErr: nil,
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
		officialErr:   fmt.Errorf("list failed"),
		installAllErr: fmt.Errorf("full install failed"),
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

func TestSyncSkills_GlobalListFailureDegradesToColdStart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialOut: officialSkillsOutput("lark-calendar", "lark-mail"),
		globalErr:   fmt.Errorf("global list failed"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatalf("SyncSkills() err = %v, want nil (degraded to cold start)", result.Err)
	}
	if result.Action != "synced" {
		t.Fatalf("SyncSkills() action = %q, want synced", result.Action)
	}
	assertStrings(t, result.Updated, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, result.SkippedDeleted, []string{})
}

func TestSyncSkills_ParseEmptyGlobalListWithNonEmptyStdoutDegradesToColdStart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialOut: officialSkillsOutput("lark-calendar", "lark-mail"),
		globalOut:   "Some unrecognized output format\n",
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Err != nil {
		t.Fatalf("SyncSkills() err = %v, want nil (degraded to cold start)", result.Err)
	}
	if result.Action != "synced" {
		t.Fatalf("SyncSkills() action = %q, want synced", result.Action)
	}
	assertStrings(t, result.Updated, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, result.SkippedDeleted, []string{})
	if runner.installedAll != 0 {
		t.Fatalf("installedAll = %d, want 0 (no fallback)", runner.installedAll)
	}
	if len(runner.installed) != 1 {
		t.Fatalf("installed = %d calls, want 1 (incremental)", len(runner.installed))
	}
}

func TestSyncSkills_InstallFailureFallsBackToFullInstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialOut:   officialSkillsOutput("lark-calendar", "lark-mail"),
		globalOut:     globalSkillsOutput("lark-calendar", "lark-mail"),
		installErr:    fmt.Errorf("incremental boom"),
		installAllErr: nil,
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
		officialOut:   officialSkillsOutput("lark-calendar", "lark-mail"),
		globalOut:     globalSkillsOutput("lark-calendar", "lark-mail"),
		installErr:    fmt.Errorf("incremental boom"),
		installAllErr: fmt.Errorf("full install boom"),
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

func TestSyncSkills_ParseEmptyWithNonEmptyStdoutFallsBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialOut:   "Some unrecognized output format\n",
		installAllErr: nil,
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
		officialOut:   "Some unrecognized output format\n",
		installAllErr: fmt.Errorf("full install failed"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Action != "fallback_failed" {
		t.Fatalf("SyncSkills() action = %q, want fallback_failed", result.Action)
	}
	if result.Err == nil {
		t.Fatalf("SyncSkills() err = nil, want error")
	}
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestSyncSkills_FallbackWithUnknownOfficialWritesMinimalState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialOut:   "Some unrecognized output format\n",
		installAllErr: nil,
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
		officialOut:   officialSkillsOutput("lark-calendar", "lark-mail"),
		globalOut:     globalSkillsOutput("lark-calendar", "lark-mail"),
		installErr:    fmt.Errorf("incremental boom"),
		installAllErr: nil,
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
		officialOut:   officialSkillsOutput("lark-calendar", "lark-mail"),
		globalOut:     globalSkillsOutput("lark-calendar", "lark-mail"),
		installErr:    fmt.Errorf("incremental boom"),
		installAllErr: nil,
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
		officialErr:   fmt.Errorf("list failed"),
		installAllErr: nil,
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
		officialOut: officialSkillsOutput("lark-calendar", "lark-mail"),
		globalOut:   globalSkillsOutput("lark-calendar", "lark-mail"),
	}
	result2 := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner2, Now: time.Now})
	if result2.Action != "synced" {
		t.Fatalf("second sync: action = %q, want synced (no fallback loop)", result2.Action)
	}
	if runner2.installedAll != 0 {
		t.Fatalf("second sync: installedAll = %d, want 0 (incremental, not fallback)", runner2.installedAll)
	}
}
