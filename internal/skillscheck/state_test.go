// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadState_Missing(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	state, ok, err := ReadState()
	if err != nil {
		t.Fatalf("ReadState() err = %v, want nil for missing file", err)
	}
	if ok {
		t.Fatal("ReadState() ok = true, want false for missing file")
	}
	if state != nil {
		t.Fatalf("ReadState() state = %#v, want nil for missing file", state)
	}
}

func TestReadState_Valid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	want := SkillsState{
		SchemaVersion:        1,
		Version:              "1.2.3",
		OfficialSkills:       []string{"lark-doc", "lark-im"},
		UpdatedSkills:        []string{"lark-doc"},
		AddedSkills:          []string{"lark-task"},
		SkippedDeletedSkills: []string{"custom-skill"},
		UpdatedAt:            "2026-05-18T10:00:00Z",
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, stateFile), data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok, err := ReadState()
	if err != nil {
		t.Fatalf("ReadState() err = %v, want nil", err)
	}
	if !ok {
		t.Fatal("ReadState() ok = false, want true")
	}
	if got == nil {
		t.Fatal("ReadState() state = nil, want state")
	}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("ReadState() state = %#v, want %#v", *got, want)
	}
}

func TestReadState_CorruptOrUnknownSchemaUnreadable(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "corrupt json", data: []byte(`{"schema_version":`)},
		{name: "unknown schema", data: []byte(`{"schema_version":2,"version":"1.2.3"}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
			if err := os.WriteFile(filepath.Join(dir, stateFile), tt.data, 0o644); err != nil {
				t.Fatal(err)
			}

			state, ok, err := ReadState()
			if err != nil {
				t.Fatalf("ReadState() err = %v, want nil", err)
			}
			if ok {
				t.Fatal("ReadState() ok = true, want false")
			}
			if state != nil {
				t.Fatalf("ReadState() state = %#v, want nil", state)
			}
		})
	}
}

func TestWriteState_CreatesDirAndWritesState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	state := SkillsState{
		Version:   "1.2.3",
		UpdatedAt: "2026-05-18T10:00:00Z",
	}
	if err := WriteState(state); err != nil {
		t.Fatalf("WriteState() err = %v, want nil", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		t.Fatal(err)
	}
	var got SkillsState
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("written state is invalid JSON: %v", err)
	}
	if got.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", got.SchemaVersion)
	}
	if got.Version != state.Version {
		t.Fatalf("version = %q, want %q", got.Version, state.Version)
	}
	if got.OfficialSkills == nil {
		t.Fatal("official_skills decoded as nil, want empty slice")
	}
	if got.UpdatedSkills == nil {
		t.Fatal("updated_skills decoded as nil, want empty slice")
	}
	if got.AddedSkills == nil {
		t.Fatal("added_skills decoded as nil, want empty slice")
	}
	if got.SkippedDeletedSkills == nil {
		t.Fatal("skipped_deleted_skills decoded as nil, want empty slice")
	}
}

func TestReadSyncedVersionFromState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	if got, ok := ReadSyncedVersion(); ok || got != "" {
		t.Fatalf("ReadSyncedVersion() = (%q, %v), want (\"\", false) for missing state", got, ok)
	}
	if err := WriteState(SkillsState{Version: "1.2.3"}); err != nil {
		t.Fatal(err)
	}
	if got, ok := ReadSyncedVersion(); !ok || got != "1.2.3" {
		t.Fatalf("ReadSyncedVersion() = (%q, %v), want (\"1.2.3\", true)", got, ok)
	}
	if err := WriteState(SkillsState{}); err != nil {
		t.Fatal(err)
	}
	if got, ok := ReadSyncedVersion(); ok || got != "" {
		t.Fatalf("ReadSyncedVersion() = (%q, %v), want (\"\", false) for empty version", got, ok)
	}
}
