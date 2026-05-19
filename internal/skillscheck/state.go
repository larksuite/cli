// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	stateFile          = "skills-state.json"
	stateSchemaVersion = 1
)

type SkillsState struct {
	SchemaVersion        int      `json:"schema_version"`
	Version              string   `json:"version"`
	OfficialSkills       []string `json:"official_skills"`
	UpdatedSkills        []string `json:"updated_skills"`
	AddedSkills          []string `json:"added_skills"`
	SkippedDeletedSkills []string `json:"skipped_deleted_skills"`
	UpdatedAt            string   `json:"updated_at"`
}

func statePath() string {
	return filepath.Join(core.GetBaseConfigDir(), stateFile)
}

func ReadState() (*SkillsState, bool, error) {
	data, err := vfs.ReadFile(statePath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}

	var state SkillsState
	if json.Unmarshal(data, &state) != nil {
		state = SkillsState{}
	}
	if state.SchemaVersion != stateSchemaVersion {
		return nil, false, nil
	}
	return &state, true, nil
}

func WriteState(state SkillsState) error {
	state.SchemaVersion = stateSchemaVersion
	state.ensureNonNilSlices()

	if err := vfs.MkdirAll(core.GetBaseConfigDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return validate.AtomicWrite(statePath(), append(data, '\n'), 0o644)
}

func ReadSyncedVersion() (string, bool) {
	state, ok, err := ReadState()
	if err != nil || !ok || state.Version == "" {
		return "", false
	}
	return state.Version, true
}

func (s *SkillsState) ensureNonNilSlices() {
	if s.OfficialSkills == nil {
		s.OfficialSkills = []string{}
	}
	if s.UpdatedSkills == nil {
		s.UpdatedSkills = []string{}
	}
	if s.AddedSkills == nil {
		s.AddedSkills = []string{}
	}
	if s.SkippedDeletedSkills == nil {
		s.SkippedDeletedSkills = []string{}
	}
}
