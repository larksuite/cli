// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skill

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/larksuite/cli/internal/cmdutil"
)

// setupFS installs a test ContentFS. Tests that touch the ContentFS package
// global must NOT call t.Parallel() — it would race.
func setupFS() {
	ContentFS = fstest.MapFS{
		"lark-calendar/SKILL.md":             {Data: []byte("---\nname: lark-calendar\ndescription: \"Cal\"\nmetadata:\n  cliHelp: \"lark-cli calendar --help\"\n---\nbody")},
		"lark-calendar/references/agenda.md": {Data: []byte("# Agenda")},
	}
}

func run(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	f, out, errOut, _ := cmdutil.TestFactory(t, nil)
	cmd := NewCmdSkill(f)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestSkillList(t *testing.T) {
	setupFS()
	stdout, _, err := run(t, "list")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	var got struct {
		OK     bool             `json:"ok"`
		Skills []map[string]any `json:"skills"`
		Count  int              `json:"count"`
	}
	if e := json.Unmarshal([]byte(stdout), &got); e != nil {
		t.Fatalf("invalid JSON: %v\n%s", e, stdout)
	}
	// "ok" marks this as a recognized envelope so _notice can be injected.
	if !got.OK {
		t.Error("expected ok=true in list envelope")
	}
	if got.Count != 1 || len(got.Skills) != 1 {
		t.Fatalf("count: got %d", got.Count)
	}
	if got.Skills[0]["name"] != "lark-calendar" {
		t.Errorf("name: got %v", got.Skills[0]["name"])
	}
	// Top-level list carries metadata, not a references list.
	if _, ok := got.Skills[0]["references"]; ok {
		t.Error("top-level list must not include references")
	}
	if _, ok := got.Skills[0]["metadata"]; !ok {
		t.Error("expected metadata in list entry")
	}
}

func TestSkillListJSONFlagAccepted(t *testing.T) {
	setupFS()
	// `list --json` must be accepted (no-op), not rejected as an unknown flag,
	// so it stays symmetric with read --json.
	stdout, _, err := run(t, "list", "--json")
	if err != nil {
		t.Fatalf("list --json error: %v", err)
	}
	var got struct {
		OK    bool `json:"ok"`
		Count int  `json:"count"`
	}
	if e := json.Unmarshal([]byte(stdout), &got); e != nil {
		t.Fatalf("invalid JSON: %v\n%s", e, stdout)
	}
	if !got.OK || got.Count != 1 {
		t.Errorf("envelope: %+v", got)
	}
}

func TestSkillListPath(t *testing.T) {
	setupFS()
	stdout, _, err := run(t, "list", "lark-calendar")
	if err != nil {
		t.Fatalf("list <name> error: %v", err)
	}
	var got struct {
		OK      bool   `json:"ok"`
		Path    string `json:"path"`
		Entries []struct {
			Path  string `json:"path"`
			IsDir bool   `json:"is_dir"`
		} `json:"entries"`
		Count int `json:"count"`
	}
	if e := json.Unmarshal([]byte(stdout), &got); e != nil {
		t.Fatalf("invalid JSON: %v\n%s", e, stdout)
	}
	if !got.OK || got.Path != "lark-calendar" {
		t.Errorf("envelope: %+v", got)
	}
	// One layer under the skill root: SKILL.md (file) + references (dir).
	if got.Count != 2 || len(got.Entries) != 2 {
		t.Fatalf("entries: got %+v", got.Entries)
	}
	if got.Entries[0].Path != "lark-calendar/SKILL.md" || got.Entries[0].IsDir {
		t.Errorf("entry[0]: got %+v", got.Entries[0])
	}
	if got.Entries[1].Path != "lark-calendar/references" || !got.Entries[1].IsDir {
		t.Errorf("entry[1]: got %+v", got.Entries[1])
	}
}

func TestSkillListPathUnknown(t *testing.T) {
	setupFS()
	_, _, err := run(t, "list", "no-such-skill")
	if err == nil || !strings.Contains(err.Error(), "unknown skill") {
		t.Fatalf("expected 'unknown skill' error, got %v", err)
	}
}

func TestSkillListPathTraversal(t *testing.T) {
	setupFS()
	stdout, _, err := run(t, "list", "lark-calendar/../../etc")
	if err == nil || !strings.Contains(err.Error(), "invalid path") {
		t.Fatalf("expected 'invalid path' error, got %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout must be empty on rejection, got %q", stdout)
	}
}

func TestSkillListTooManyArgs(t *testing.T) {
	setupFS()
	_, _, err := run(t, "list", "a", "b")
	if err == nil || !strings.Contains(err.Error(), "at most 1 argument") {
		t.Fatalf("expected 'at most 1 argument' error, got %v", err)
	}
}

func TestSkillReadRaw(t *testing.T) {
	setupFS()
	stdout, _, err := run(t, "read", "lark-calendar")
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if !strings.HasPrefix(stdout, "---\nname: lark-calendar") {
		t.Errorf("raw output: got %q", stdout)
	}
	// Main SKILL.md output appends a guidance tip after the content.
	if !strings.Contains(stdout, "lark-cli skills read lark-calendar <path>") {
		t.Errorf("expected guidance tip in raw output: got %q", stdout)
	}
}

func TestSkillReadJSON(t *testing.T) {
	setupFS()
	stdout, _, err := run(t, "read", "lark-calendar", "--json")
	if err != nil {
		t.Fatalf("read --json error: %v", err)
	}
	var got struct {
		Skill, Path, Content, Guidance string
	}
	if e := json.Unmarshal([]byte(stdout), &got); e != nil {
		t.Fatalf("invalid JSON: %v", e)
	}
	if got.Skill != "lark-calendar" || got.Path != "SKILL.md" || got.Content == "" {
		t.Errorf("envelope: %+v", got)
	}
	// Guidance is a separate field, not merged into content.
	if got.Guidance == "" {
		t.Error("expected guidance field for main SKILL.md")
	}
	if strings.Contains(got.Content, "Tip:") {
		t.Error("guidance must not be merged into content")
	}
}

func TestSkillReadFile(t *testing.T) {
	setupFS()
	// Both the 2-arg and slash forms read the same file, with no guidance tip.
	for _, args := range [][]string{
		{"read", "lark-calendar", "references/agenda.md"},
		{"read", "lark-calendar/references/agenda.md"},
	} {
		stdout, _, err := run(t, args...)
		if err != nil {
			t.Fatalf("read %v error: %v", args, err)
		}
		if stdout != "# Agenda" {
			t.Errorf("read %v output: got %q", args, stdout)
		}
	}
}

func TestSkillReadFileJSON(t *testing.T) {
	setupFS()
	stdout, _, err := run(t, "read", "lark-calendar", "references/agenda.md", "--json")
	if err != nil {
		t.Fatalf("read file --json error: %v", err)
	}
	var got struct {
		Skill, Path, Content, Guidance string
	}
	if e := json.Unmarshal([]byte(stdout), &got); e != nil {
		t.Fatalf("invalid JSON: %v\n%s", e, stdout)
	}
	if got.Skill != "lark-calendar" || got.Path != "references/agenda.md" || got.Content != "# Agenda" {
		t.Errorf("envelope: %+v", got)
	}
	// Reference reads do not carry the guidance tip.
	if got.Guidance != "" {
		t.Errorf("reference read must not include guidance, got %q", got.Guidance)
	}
}

func TestSkillReadUnknown(t *testing.T) {
	setupFS()
	_, _, err := run(t, "read", "no-such")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown skill") {
		t.Errorf("err: %v", err)
	}
}

func TestSkillReadMissingArg(t *testing.T) {
	setupFS()
	_, _, err := run(t, "read")
	if err == nil || !strings.Contains(err.Error(), "requires 1 or 2 arguments") {
		t.Fatalf("expected arg error, got %v", err)
	}
}

func TestSkillReadTraversal(t *testing.T) {
	setupFS()
	stdout, _, err := run(t, "read", "lark-calendar", "../../etc/passwd")
	if err == nil {
		t.Fatal("expected rejection")
	}
	if !strings.Contains(err.Error(), "invalid path") {
		t.Errorf("err: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout must be empty on rejection, got %q", stdout)
	}
}

func TestSkillNilContentFS(t *testing.T) {
	ContentFS = nil
	t.Cleanup(setupFS)
	_, _, err := run(t, "list")
	if err == nil {
		t.Fatal("expected error when ContentFS is nil")
	}
}
