// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	stateFile = "skills-state.json"
)

var ErrUnreadableState = errors.New("skills state is unreadable")

type SkillsState struct {
	Version               string   `json:"version"`
	Layout                Layout   `json:"layout,omitempty"`
	OfficialSkills        []string `json:"official_skills"`
	OfficialSkillsUnknown bool     `json:"official_skills_unknown,omitempty"`
	UpdatedSkills         []string `json:"updated_skills"`
	AddedOfficialSkills   []string `json:"added_official_skills"`
	SkippedDeletedSkills  []string `json:"skipped_deleted_skills"`
	UpdatedAt             string   `json:"updated_at"`
}

// KnownOfficialSkills returns the previous managed Skill set when the state is
// authoritative. Callers receive a copy so installation planning cannot mutate
// the persisted state in memory.
func KnownOfficialSkills(state *SkillsState) []string {
	if state == nil || state.OfficialSkillsUnknown {
		return nil
	}
	return slices.Clone(state.OfficialSkills)
}

// NewCompleteState builds state for a complete managed Skills replacement.
// Every supplied Skill is installed in this operation, and Skills that were not
// present in the previous authoritative state are recorded as newly added.
func NewCompleteState(version string, layout Layout, official []string, previous *SkillsState) SkillsState {
	official = slices.Clone(official)
	previousSet := make(map[string]bool)
	for _, name := range KnownOfficialSkills(previous) {
		previousSet[name] = true
	}
	added := make([]string, 0, len(official))
	for _, name := range official {
		if !previousSet[name] {
			added = append(added, name)
		}
	}
	state := SkillsState{
		Version:             version,
		Layout:              layout,
		OfficialSkills:      official,
		UpdatedSkills:       slices.Clone(official),
		AddedOfficialSkills: added,
		UpdatedAt:           time.Now().UTC().Format(time.RFC3339),
	}
	state.ensureNonNilSlices()
	return state
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

// SnapshotState captures the exact state file and returns a restore function.
// Distribution installation uses it to roll back a state write together with
// the managed Skills directories when a later binary replacement fails.
func SnapshotState() (restore func() error, err error) {
	path := statePath()
	data, readErr := vfs.ReadFile(path)
	if readErr != nil {
		if !errors.Is(readErr, fs.ErrNotExist) {
			return nil, readErr
		}
		return func() error {
			removeErr := vfs.Remove(path)
			if errors.Is(removeErr, fs.ErrNotExist) {
				return nil
			}
			return removeErr
		}, nil
	}
	return func() error {
		if err := vfs.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		return validate.AtomicWrite(path, data, 0o644)
	}, nil
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
