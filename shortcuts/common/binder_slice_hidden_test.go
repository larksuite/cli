// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// --- multi-value ([]string) flags -----------------------------------------

type sliceArgs struct {
	Ids  []string `flag:"ids"`               // default → StringSlice (comma-split)
	Tags []string `flag:"tags" split:"none"` // StringArray (repeatable, no split)
}

func TestSliceFlag_StringSliceDefault(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, err := walkArgs(reflect.TypeOf(&sliceArgs{}))
	if err != nil {
		t.Fatalf("walkArgs: %v", err)
	}
	if err := registerFlags(cmd, specs); err != nil {
		t.Fatalf("registerFlags: %v", err)
	}
	if err := cmd.ParseFlags([]string{"--ids", "a,b,c"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := &sliceArgs{}
	if err := bindFlags(cmd, reflect.ValueOf(out).Elem(), specs); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if !reflect.DeepEqual(out.Ids, []string{"a", "b", "c"}) {
		t.Errorf("Ids = %#v, want [a b c] (comma-split)", out.Ids)
	}
}

func TestSliceFlag_StringArrayNoSplit(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&sliceArgs{}))
	_ = registerFlags(cmd, specs)
	// repeated; a value containing a comma must NOT be split (StringArray)
	if err := cmd.ParseFlags([]string{"--tags", "a,b", "--tags", "c"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := &sliceArgs{}
	_ = bindFlags(cmd, reflect.ValueOf(out).Elem(), specs)
	if !reflect.DeepEqual(out.Tags, []string{"a,b", "c"}) {
		t.Errorf("Tags = %#v, want [\"a,b\" \"c\"] (no split, repeatable)", out.Tags)
	}
}

func TestSliceFlag_UnsetIsEmpty(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&sliceArgs{}))
	_ = registerFlags(cmd, specs)
	_ = cmd.ParseFlags(nil)
	out := &sliceArgs{}
	_ = bindFlags(cmd, reflect.ValueOf(out).Elem(), specs)
	if len(out.Ids) != 0 {
		t.Errorf("Ids = %#v, want empty when unset", out.Ids)
	}
}

type sliceGroup struct {
	Items []string `flag:"items"`
	Note  string   `flag:"note"`
}

type groupSliceArgs struct {
	G sliceGroup
}

func TestSliceFlag_InGroup(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, err := walkArgs(reflect.TypeOf(&groupSliceArgs{}))
	if err != nil {
		t.Fatalf("walkArgs: %v", err)
	}
	_ = registerFlags(cmd, specs)
	if err := cmd.ParseFlags([]string{"--items", "x,y", "--note", "hi"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := &groupSliceArgs{}
	if err := bindGroups(cmd, reflect.ValueOf(out).Elem(), specs); err != nil {
		t.Fatalf("bindGroups: %v", err)
	}
	if !reflect.DeepEqual(out.G.Items, []string{"x", "y"}) {
		t.Errorf("G.Items = %#v, want [x y]", out.G.Items)
	}
}

// --- Mount-time validation for slices / split -----------------------------

type splitOnStringArgs struct {
	S string `flag:"s" split:"none"`
}

func TestWalkArgs_SplitOnNonSliceErrors(t *testing.T) {
	_, err := walkArgs(reflect.TypeOf(&splitOnStringArgs{}))
	if err == nil || !strings.Contains(err.Error(), "split tag is only supported on []string") {
		t.Fatalf("expected split-on-non-slice error, got %v", err)
	}
}

type intSliceArgs struct {
	N []int `flag:"n"`
}

func TestWalkArgs_NonStringSliceErrors(t *testing.T) {
	_, err := walkArgs(reflect.TypeOf(&intSliceArgs{}))
	if err == nil || !strings.Contains(err.Error(), "only []string slices are supported") {
		t.Fatalf("expected []int error, got %v", err)
	}
}

type unknownSplitArgs struct {
	S []string `flag:"s" split:"bogus"`
}

func TestWalkArgs_UnknownSplitMode(t *testing.T) {
	_, err := walkArgs(reflect.TypeOf(&unknownSplitArgs{}))
	if err == nil || !strings.Contains(err.Error(), "unknown split mode") {
		t.Fatalf("expected unknown split mode error, got %v", err)
	}
}

// --- per-flag hidden ------------------------------------------------------

type hiddenArgs struct {
	Visible string `flag:"visible"`
	Secret  string `flag:"secret" hidden:"true"`
}

func TestHiddenFlag_RegisteredButHidden(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&hiddenArgs{}))
	_ = registerFlags(cmd, specs)
	f := cmd.Flags().Lookup("secret")
	if f == nil {
		t.Fatal("secret flag not registered")
	}
	if !f.Hidden {
		t.Error("secret flag should be marked hidden")
	}
	// hidden does not mean disabled — it still binds.
	_ = cmd.ParseFlags([]string{"--secret", "shh", "--visible", "v"})
	out := &hiddenArgs{}
	_ = bindFlags(cmd, reflect.ValueOf(out).Elem(), specs)
	if out.Secret != "shh" {
		t.Errorf("Secret = %q, want shh (hidden but functional)", out.Secret)
	}
}

func TestHiddenFlag_SkippedInHelp(t *testing.T) {
	specs, _ := walkArgs(reflect.TypeOf(&hiddenArgs{}))
	cmd := &cobra.Command{Use: "test"}
	_ = registerFlags(cmd, specs)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	buildTypedHelp(specs, nil)(cmd, nil)
	out := buf.String()
	if !strings.Contains(out, "--visible") {
		t.Errorf("help should show --visible:\n%s", out)
	}
	if strings.Contains(out, "--secret") {
		t.Errorf("help must NOT show hidden --secret:\n%s", out)
	}
}

// --- @file / stdin help hint ----------------------------------------------

func TestFormatLeafLine_InputHintBoth(t *testing.T) {
	s := fieldSpec{FlagName: "content", Description: "the content", Input: []string{File, Stdin}}
	line := formatLeafLine("  ", s)
	if !strings.Contains(line, "(supports @file, - for stdin)") {
		t.Errorf("missing input hint: %q", line)
	}
}

func TestFormatLeafLine_InputHintFileOnly(t *testing.T) {
	s := fieldSpec{FlagName: "content", Description: "c", Input: []string{File}}
	line := formatLeafLine("  ", s)
	if !strings.Contains(line, "(supports @file)") || strings.Contains(line, "stdin") {
		t.Errorf("file-only hint wrong: %q", line)
	}
}
