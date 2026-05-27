// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	stateFile = "skills-state.json"
)

var ErrUnreadableState = errors.New("skills state is unreadable")

type SkillsState struct {
	Version              string   `json:"version"`
	OfficialSkills       []string `json:"official_skills"`
	UpdatedSkills        []string `json:"updated_skills"`
	AddedOfficialSkills  []string `json:"added_official_skills"`
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

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrUnreadableState, err)
	}

	var state SkillsState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrUnreadableState, err)
	}
	return &state, true, nil
}

func WriteState(state SkillsState) error {
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
	if s.AddedOfficialSkills == nil {
		s.AddedOfficialSkills = []string{}
	}
	if s.SkippedDeletedSkills == nil {
		s.SkippedDeletedSkills = []string{}
	}
}
