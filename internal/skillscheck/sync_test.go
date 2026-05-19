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

func TestParseSkillsList(t *testing.T) {
	input := `Installed skills:
- lark-calendar
- lark-mail
lark-im
custom-skill
lark-base@1.0.0
lark-cli-harness:dev@0.1.0
`
	got := ParseSkillsList(input)
	want := []string{"custom-skill", "lark-base", "lark-calendar", "lark-cli-harness:dev", "lark-im", "lark-mail"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSkillsList() = %#v, want %#v", got, want)
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
	officialOut string
	globalOut   string
	officialErr error
	globalErr   error
	installErr  map[string]error
	installed   []string
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

func (f *fakeSkillsRunner) InstallSkill(name string) *selfupdate.NpmResult {
	f.installed = append(f.installed, name)
	r := &selfupdate.NpmResult{}
	if f.installErr != nil {
		r.Err = f.installErr[name]
	}
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
		officialOut: "lark-calendar\nlark-mail\nlark-new\n",
		globalOut:   "lark-calendar\nlark-custom\n",
	}
	result := SyncSkills(SyncOptions{
		Version: "1.0.33",
		Runner:  runner,
		Now:     func() time.Time { return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC) },
	})

	if result.Err != nil {
		t.Fatalf("SyncSkills() err = %v, want nil", result.Err)
	}
	assertStrings(t, runner.installed, []string{"lark-calendar", "lark-new"})

	state, readable, err := ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	assertStrings(t, state.OfficialSkills, []string{"lark-calendar", "lark-mail", "lark-new"})
	assertStrings(t, state.UpdatedSkills, []string{"lark-calendar", "lark-new"})
	assertStrings(t, state.AddedSkills, []string{"lark-new"})
	assertStrings(t, state.SkippedDeletedSkills, []string{"lark-mail"})
	if _, err := os.Stat(filepath.Join(dir, "skills.stamp")); !os.IsNotExist(err) {
		t.Fatalf("skills.stamp exists or stat failed with unexpected err: %v", err)
	}
}

func TestSyncSkills_ListFailureDoesNotInstallOrWriteState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{officialErr: fmt.Errorf("list failed")}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Err == nil || !strings.Contains(result.Err.Error(), "failed to list official skills") {
		t.Fatalf("SyncSkills() err = %v, want official list failure", result.Err)
	}
	if len(runner.installed) != 0 {
		t.Fatalf("installed = %#v, want none", runner.installed)
	}
	if _, readable, err := ReadState(); err != nil || readable {
		t.Fatalf("ReadState() = (_, %v, %v), want unreadable missing state", readable, err)
	}
}

func TestSyncSkills_GlobalListFailureDoesNotInstallOrWriteState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialOut: "lark-calendar\nlark-mail\n",
		globalErr:   fmt.Errorf("global list failed"),
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Err == nil || !strings.Contains(result.Err.Error(), "failed to list installed skills") {
		t.Fatalf("SyncSkills() err = %v, want installed list failure", result.Err)
	}
	if len(runner.installed) != 0 {
		t.Fatalf("installed = %#v, want none", runner.installed)
	}
	if _, readable, err := ReadState(); err != nil || readable {
		t.Fatalf("ReadState() = (_, %v, %v), want unreadable missing state", readable, err)
	}
}

func TestSyncSkills_InstallFailureContinuesAndDoesNotWriteState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	runner := &fakeSkillsRunner{
		officialOut: "lark-calendar\nlark-mail\n",
		globalOut:   "lark-calendar\nlark-mail\n",
		installErr:  map[string]error{"lark-calendar": fmt.Errorf("boom")},
	}

	result := SyncSkills(SyncOptions{Version: "1.0.33", Runner: runner, Now: time.Now})
	if result.Err == nil || !strings.Contains(result.Err.Error(), "1 skill(s) failed") {
		t.Fatalf("SyncSkills() err = %v, want install failure", result.Err)
	}
	assertStrings(t, runner.installed, []string{"lark-calendar", "lark-mail"})
	assertStrings(t, result.Failed, []string{"lark-calendar"})
	if !strings.Contains(result.Detail, "boom") {
		t.Fatalf("SyncSkills() detail = %q, want install error text", result.Detail)
	}
	if _, readable, err := ReadState(); err != nil || readable {
		t.Fatalf("ReadState() = (_, %v, %v), want no success state", readable, err)
	}
}

func TestSyncSkills_NilRunnerFails(t *testing.T) {
	result := SyncSkills(SyncOptions{Version: "1.0.33", Now: time.Now})
	if result.Err == nil || !strings.Contains(result.Err.Error(), "skills runner is nil") {
		t.Fatalf("SyncSkills() err = %v, want nil runner failure", result.Err)
	}
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
