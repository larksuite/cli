// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// State is the JSON document at <data>/state.json describing the currently
// installed lark-sec-cli artifact. It is the source of truth for what binary
// to launch. After bootstrap install lark-sec-cli may upgrade itself in
// place — when that happens this state file is informational only; the
// daemon owns its own canonical version state.
type State struct {
	Version     string    `json:"version"`
	BuildID     string    `json:"build_id"`
	InstalledAt time.Time `json:"installed_at"`
	BinaryPath  string    `json:"binary_path"`
}

// LoadState reads state.json. Returns (nil, nil) when the file is absent —
// callers treat that as "not yet installed". Decode errors are surfaced
// so a corrupt file is never silently overwritten.
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &s, nil
}

// SaveState writes state.json atomically: a tmpfile next to the target is
// fsynced then renamed in, so concurrent readers either see the previous
// state or the new one — never a torn write.
func SaveState(path string, s *State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dirOf(path), ".state-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful Rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
