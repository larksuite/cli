// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skill

import (
	"encoding/json"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/larksuite/cli/internal/cmdutil"
)

// calFS is the default single-skill content tree for these tests. The embedded
// FS is now injected through the Factory (no package global), so tests pass it
// explicitly to run() — nothing is shared, so they are safe under -parallel.
func calFS() fstest.MapFS {
	return fstest.MapFS{
		"lark-calendar/SKILL.md":             {Data: []byte("---\nname: lark-calendar\nversion: 1.0.0\ndescription: \"Cal\"\nmetadata:\n  cliHelp: \"lark-cli calendar --help\"\n---\nbody")},
		"lark-calendar/references/agenda.md": {Data: []byte("# Agenda")},
	}
}

// run executes the skills command tree against the given content FS (may be nil
// to exercise the not-embedded path) and returns stdout/stderr/err.
func run(t *testing.T, fsys fs.FS, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	// Isolate CLI config state so tests never read/write the real config dir
	// (repo convention).
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, out, errOut, _ := cmdutil.TestFactory(t, nil)
	f.SkillContent = fsys
	cmd := NewCmdSkill(f)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestSkillList(t *testing.T) {
	stdout, _, err := run(t, calFS(), "list")
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
	// "ok" is an explicit success marker (the list envelope is a typed struct;
	// no automatic _notice attaches).
	if !got.OK {
		t.Error("expected ok=true in list envelope")
	}
	if got.Count != 1 || len(got.Skills) != 1 {
		t.Fatalf("count: got %d", got.Count)
	}
	if got.Skills[0]["name"] != "lark-calendar" {
		t.Errorf("name: got %v", got.Skills[0]["name"])
	}
	// Top-level list carries version + metadata, not a references list.
	if _, ok := got.Skills[0]["references"]; ok {
		t.Error("top-level list must not include references")
	}
	if got.Skills[0]["version"] != "1.0.0" {
		t.Errorf("version: got %v, want 1.0.0", got.Skills[0]["version"])
	}
	if _, ok := got.Skills[0]["metadata"]; !ok {
		t.Error("expected metadata in list entry")
	}
}

func TestSkillListJSONFlagAccepted(t *testing.T) {
	// `list --json` must be accepted (no-op), not rejected as an unknown flag,
	// so it stays symmetric with read --json.
	stdout, _, err := run(t, calFS(), "list", "--json")
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
	stdout, _, err := run(t, calFS(), "list", "lark-calendar")
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
	_, _, err := run(t, calFS(), "list", "no-such-skill")
	if err == nil || !strings.Contains(err.Error(), "unknown skill") {
		t.Fatalf("expected 'unknown skill' error, got %v", err)
	}
}

func TestSkillListPathTraversal(t *testing.T) {
	stdout, _, err := run(t, calFS(), "list", "lark-calendar/../../etc")
	if err == nil || !strings.Contains(err.Error(), "invalid path") {
		t.Fatalf("expected 'invalid path' error, got %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout must be empty on rejection, got %q", stdout)
	}
}

func TestSkillListTooManyArgs(t *testing.T) {
	_, _, err := run(t, calFS(), "list", "a", "b")
	if err == nil || !strings.Contains(err.Error(), "at most 1 argument") {
		t.Fatalf("expected 'at most 1 argument' error, got %v", err)
	}
}

// TestSkillListSkipsDirWithoutSKILLmd proves a top-level dir lacking SKILL.md is
// omitted from the catalog (no blank entry).
func TestSkillListSkipsDirWithoutSKILLmd(t *testing.T) {
	fsys := fstest.MapFS{
		"lark-calendar/SKILL.md": {Data: []byte("---\nname: lark-calendar\ndescription: \"Cal\"\n---\nb")},
		"not-a-skill/readme.txt": {Data: []byte("junk")}, // dir without SKILL.md
	}
	stdout, _, err := run(t, fsys, "list")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	var got struct {
		Skills []map[string]any `json:"skills"`
		Count  int              `json:"count"`
	}
	if e := json.Unmarshal([]byte(stdout), &got); e != nil {
		t.Fatalf("invalid JSON: %v\n%s", e, stdout)
	}
	if got.Count != 1 || got.Skills[0]["name"] != "lark-calendar" {
		t.Fatalf("expected only lark-calendar, got %+v", got.Skills)
	}
}

func TestSkillReadRaw(t *testing.T) {
	stdout, stderr, err := run(t, calFS(), "read", "lark-calendar")
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if !strings.HasPrefix(stdout, "---\nname: lark-calendar") {
		t.Errorf("raw output: got %q", stdout)
	}
	// Raw stdout is byte-pure SKILL.md — the guidance tip must NOT be appended.
	if strings.Contains(stdout, "Tip:") {
		t.Errorf("raw stdout must not carry the guidance tip: got %q", stdout)
	}
	// Guidance goes to stderr: own files via `skills read <name> ...`, and
	// cross-skill refs routed to `skills read <other-skill> ...` (version-
	// consistent), not "read directly".
	if !strings.Contains(stderr, "lark-cli skills read lark-calendar <relative-path>") {
		t.Errorf("expected own-files guidance on stderr: got %q", stderr)
	}
	if !strings.Contains(stderr, "lark-cli skills read lark-foo/...") {
		t.Errorf("expected cross-skill refs routed to skills read: got %q", stderr)
	}
	if strings.Contains(stderr, "instead of opening them directly") ||
		strings.Contains(stderr, "read those directly") {
		t.Errorf("guidance must not steer cross-skill refs to direct reads: got %q", stderr)
	}
}

func TestSkillReadJSON(t *testing.T) {
	stdout, _, err := run(t, calFS(), "read", "lark-calendar", "--json")
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
	// Both the 2-arg and slash forms read the same file, with no guidance tip.
	for _, args := range [][]string{
		{"read", "lark-calendar", "references/agenda.md"},
		{"read", "lark-calendar/references/agenda.md"},
	} {
		stdout, stderr, err := run(t, calFS(), args...)
		if err != nil {
			t.Fatalf("read %v error: %v", args, err)
		}
		if stdout != "# Agenda" {
			t.Errorf("read %v output: got %q", args, stdout)
		}
		// Reference reads carry no guidance on either stream.
		if strings.Contains(stderr, "Tip:") {
			t.Errorf("read %v must not emit guidance on stderr: got %q", args, stderr)
		}
	}
}

func TestSkillReadFileJSON(t *testing.T) {
	stdout, _, err := run(t, calFS(), "read", "lark-calendar", "references/agenda.md", "--json")
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
	_, _, err := run(t, calFS(), "read", "no-such")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown skill") {
		t.Errorf("err: %v", err)
	}
}

func TestSkillReadMissingArg(t *testing.T) {
	_, _, err := run(t, calFS(), "read")
	if err == nil || !strings.Contains(err.Error(), "requires 1 or 2 arguments") {
		t.Fatalf("expected arg error, got %v", err)
	}
}

func TestSkillReadTraversal(t *testing.T) {
	stdout, _, err := run(t, calFS(), "read", "lark-calendar", "../../etc/passwd")
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
	_, _, err := run(t, nil, "list")
	if err == nil {
		t.Fatal("expected error when SkillContent is nil")
	}
	if !strings.Contains(err.Error(), "not embedded") {
		t.Errorf("err: %v", err)
	}
}
