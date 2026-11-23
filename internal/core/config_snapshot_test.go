// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"errors"
	"io/fs"
	"testing"
)

func TestConfigSnapshotLoadsOnce(t *testing.T) {
	calls := 0
	want := &MultiAppConfig{}
	snapshot := newConfigSnapshot(func() (*MultiAppConfig, error) {
		calls++
		return want, nil
	})

	for range 2 {
		config, err := snapshot.MultiAppConfig()
		if err != nil {
			t.Fatal(err)
		}
		if config != want {
			t.Fatal("snapshot returned a different config instance")
		}
	}
	if calls != 1 {
		t.Fatalf("config loads = %d, want 1", calls)
	}
}

func TestConfigSnapshotZeroValueIsMissing(t *testing.T) {
	config, err := (&ConfigSnapshot{}).MultiAppConfig()
	if config != nil || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("MultiAppConfig() = (%v, %v), want (nil, fs.ErrNotExist)", config, err)
	}
}

func TestConfigSnapshotCachesError(t *testing.T) {
	calls := 0
	want := errors.New("load failed")
	snapshot := newConfigSnapshot(func() (*MultiAppConfig, error) {
		calls++
		return nil, want
	})

	for range 2 {
		config, err := snapshot.MultiAppConfig()
		if config != nil || !errors.Is(err, want) {
			t.Fatalf("MultiAppConfig() = (%v, %v), want (nil, %v)", config, err, want)
		}
	}
	if calls != 1 {
		t.Fatalf("config loads = %d, want 1", calls)
	}
}
