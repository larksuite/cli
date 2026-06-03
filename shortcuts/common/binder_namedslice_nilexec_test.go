// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
)

// --- []<named string> and named slice types bind without panicking --------
// Regression guard: reflect.Convert([]string -> []myID) panics, so the binder
// must build the slice element-by-element via SetString instead.

type myID string

type namedElemArgs struct {
	Xs []myID `flag:"xs"`
}

func TestSliceFlag_NamedElementType(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, err := walkArgs(reflect.TypeOf(&namedElemArgs{}))
	if err != nil {
		t.Fatalf("walkArgs: %v", err)
	}
	if err := registerFlags(cmd, specs); err != nil {
		t.Fatalf("registerFlags: %v", err)
	}
	if err := cmd.ParseFlags([]string{"--xs", "a,b"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := &namedElemArgs{}
	if err := bindFlags(cmd, reflect.ValueOf(out).Elem(), specs); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if !reflect.DeepEqual(out.Xs, []myID{"a", "b"}) {
		t.Errorf("Xs = %#v, want []myID{a b}", out.Xs)
	}
}

type myIDList []string

type namedSliceArgs struct {
	Ids myIDList `flag:"ids"`
}

func TestSliceFlag_NamedSliceType(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&namedSliceArgs{}))
	_ = registerFlags(cmd, specs)
	_ = cmd.ParseFlags([]string{"--ids", "x,y,z"})
	out := &namedSliceArgs{}
	if err := bindFlags(cmd, reflect.ValueOf(out).Elem(), specs); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if !reflect.DeepEqual(out.Ids, myIDList{"x", "y", "z"}) {
		t.Errorf("Ids = %#v, want myIDList{x y z}", out.Ids)
	}
}

// --- nil Execute → not mounted (parity with legacy Shortcut) ---------------

type nilExecArgs struct {
	Name string `flag:"name"`
}

func TestMountTyped_NilExecuteNotMounted(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	ts := TypedShortcut[*nilExecArgs]{
		Service: "x", Command: "+noexec", AuthTypes: []string{"user"}, Risk: "read",
		// Execute intentionally nil — legacy skips mounting such shortcuts.
	}
	ts.MountWithContext(context.Background(), root, &cmdutil.Factory{})
	if sub, _, _ := root.Find([]string{"+noexec"}); sub != nil && sub.Name() == "+noexec" {
		t.Error("nil-Execute typed shortcut must NOT be mounted (parity with legacy)")
	}
}

func TestMountTyped_WithExecuteMounted(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	ts := TypedShortcut[*nilExecArgs]{
		Service: "x", Command: "+yesexec", AuthTypes: []string{"user"}, Risk: "read",
		Execute: func(ctx context.Context, args *nilExecArgs, rt *RuntimeContext) error { return nil },
	}
	ts.MountWithContext(context.Background(), root, &cmdutil.Factory{})
	if sub, _, _ := root.Find([]string{"+yesexec"}); sub == nil || sub.Name() != "+yesexec" {
		t.Error("typed shortcut with Execute should be mounted")
	}
}
