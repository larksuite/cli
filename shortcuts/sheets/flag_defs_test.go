// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

// TestFlagDefs_EmbedParses asserts the embedded flag-defs.json blob is valid
// JSON with at least one command entry.
func TestFlagDefs_EmbedParses(t *testing.T) {
	t.Parallel()
	defs, err := loadFlagDefs()
	if err != nil {
		t.Fatalf("loadFlagDefs error: %v", err)
	}
	if len(defs) == 0 {
		t.Fatal("flag-defs.json has no command entries")
	}
}

// TestFlagsFor_SkipsSystemFlags verifies system-kind flags (--dry-run, --yes)
// are never materialized into a shortcut's Flags slice — the framework injects
// those based on Risk / DryRun.
func TestFlagsFor_SkipsSystemFlags(t *testing.T) {
	t.Parallel()
	for _, cmd := range []string{"+sheet-delete", "+batch-update", "+csv-get"} {
		for _, f := range flagsFor(cmd) {
			if f.Name == "dry-run" || f.Name == "yes" {
				t.Errorf("%s: system flag --%s leaked into Flags", cmd, f.Name)
			}
		}
	}
}

// TestFlagsFor_MapsAllFields spot-checks that name/type/default/enum/input/
// required/hidden are carried over from the JSON correctly.
func TestFlagsFor_MapsAllFields(t *testing.T) {
	t.Parallel()
	byName := func(cmd, name string) *common.Flag {
		flags := flagsFor(cmd)
		for i := range flags {
			if flags[i].Name == name {
				return &flags[i]
			}
		}
		return nil
	}

	// enum + default
	rt := byName("+dim-insert", "inherit-style")
	if rt == nil || len(rt.Enum) != 3 || rt.Default != "none" {
		t.Errorf("+dim-insert --inherit-style not mapped: %+v", rt)
	}
	// required
	title := byName("+sheet-create", "title")
	if title == nil || !title.Required {
		t.Errorf("+sheet-create --title should be required: %+v", title)
	}
	// xor is NOT cobra-required (enforced by Validate hooks)
	url := byName("+sheet-create", "url")
	if url == nil || url.Required {
		t.Errorf("+sheet-create --url should not be cobra-required: %+v", url)
	}
	// hidden + int default
	cap := byName("+cells-get", "cell-limit")
	if cap == nil || !cap.Hidden || cap.Default != "5000" {
		t.Errorf("+cells-get --cell-limit not mapped: %+v", cap)
	}
	// input sources
	cells := byName("+cells-set", "cells")
	if cells == nil || len(cells.Input) != 2 {
		t.Errorf("+cells-set --cells should support file+stdin: %+v", cells)
	}
	// float64 type
	fs := byName("+cells-set-style", "font-size")
	if fs == nil || fs.Type != "float64" {
		t.Errorf("+cells-set-style --font-size should be float64: %+v", fs)
	}
}

// TestFlagsFor_EveryRegisteredCommandHasDefs ensures every shortcut returned by
// Shortcuts() has a flag-defs.json entry and that its flags match the JSON's
// non-system flags exactly (name + type + required + default + hidden). This is
// the contract that lets shortcuts drop hand-written flag literals.
func TestFlagsFor_EveryRegisteredCommandHasDefs(t *testing.T) {
	t.Parallel()
	defs, err := loadFlagDefs()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range Shortcuts() {
		spec, ok := defs[s.Command]
		if !ok {
			t.Errorf("%s has no flag-defs.json entry", s.Command)
			continue
		}
		want := map[string]flagDef{}
		for _, df := range spec.Flags {
			if df.Kind != "system" {
				want[df.Name] = df
			}
		}
		got := map[string]bool{}
		for _, f := range s.Flags {
			got[f.Name] = true
			df, ok := want[f.Name]
			if !ok {
				t.Errorf("%s --%s present in Go but not in JSON (non-system)", s.Command, f.Name)
				continue
			}
			ft := f.Type
			if ft == "" {
				ft = "string"
			}
			jt := df.Type
			if jt == "" {
				jt = "string"
			}
			if ft != jt {
				t.Errorf("%s --%s type: go=%s json=%s", s.Command, f.Name, ft, jt)
			}
			if f.Required != (df.Required == "required") {
				t.Errorf("%s --%s required: go=%v json=%s", s.Command, f.Name, f.Required, df.Required)
			}
			if f.Default != df.Default {
				t.Errorf("%s --%s default: go=%q json=%q", s.Command, f.Name, f.Default, df.Default)
			}
			if f.Hidden != df.Hidden {
				t.Errorf("%s --%s hidden: go=%v json=%v", s.Command, f.Name, f.Hidden, df.Hidden)
			}
		}
		for name := range want {
			if !got[name] {
				t.Errorf("%s --%s in JSON but missing from Go Flags", s.Command, name)
			}
		}
	}
}
