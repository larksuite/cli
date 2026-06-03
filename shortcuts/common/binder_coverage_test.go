// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
)

// --- top-level pointer leaf: nil = not given (mirrors OneOf bucket convention) ---

type ptrLeafArgs struct {
	Notify *bool   `flag:"notify"`
	Limit  *int    `flag:"limit"`
	Name   *string `flag:"name"`
}

func TestBindLeaf_PtrNilWhenAbsent(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&ptrLeafArgs{}))
	if err := registerFlags(cmd, specs); err != nil {
		t.Fatalf("registerFlags: %v", err)
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	out := &ptrLeafArgs{}
	if err := bindFlags(cmd, reflect.ValueOf(out).Elem(), specs); err != nil {
		t.Fatalf("bindFlags: %v", err)
	}
	if out.Notify != nil || out.Limit != nil || out.Name != nil {
		t.Errorf("expected all pointer leaves nil when no flag given; got Notify=%v Limit=%v Name=%v",
			out.Notify, out.Limit, out.Name)
	}
}

// TestBindLeaf_PtrSetWhenChanged covers the tri-state contract that previously
// required Maybe[T]: a pointer leaf set to its zero value (e.g. --notify=false)
// MUST come back non-nil so business code can tell it apart from "not given".
func TestBindLeaf_PtrSetWhenChanged(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&ptrLeafArgs{}))
	_ = registerFlags(cmd, specs)
	if err := cmd.ParseFlags([]string{"--notify=false", "--limit", "5", "--name", "alice"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	out := &ptrLeafArgs{}
	if err := bindFlags(cmd, reflect.ValueOf(out).Elem(), specs); err != nil {
		t.Fatalf("bindFlags: %v", err)
	}
	if out.Notify == nil || *out.Notify != false {
		t.Errorf("Notify = %v, want non-nil false (tri-state: zero value is still 'set')", out.Notify)
	}
	if out.Limit == nil || *out.Limit != 5 {
		t.Errorf("Limit = %v, want non-nil 5", out.Limit)
	}
	if out.Name == nil || *out.Name != "alice" {
		t.Errorf("Name = %v, want non-nil alice", out.Name)
	}
}

// --- OneOf bucket binding: bindBuckets / bindBucketInner / bucketLeafValue ----

type bcLeafBucket struct {
	S     *string `flag:"lb-s"`
	I     *int    `flag:"lb-i"`
	B     *bool   `flag:"lb-b"`
	Plain string  `flag:"lb-plain"` // non-pointer leaf exercises the value branch
}

func (bcLeafBucket) OneOf() {}

type bcValueBucketArgs struct {
	Sel bcLeafBucket
}

func TestBindBuckets_ValueBucketTypedLeaves(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&bcValueBucketArgs{}))
	if err := registerFlags(cmd, specs); err != nil {
		t.Fatalf("registerFlags: %v", err)
	}
	if err := cmd.ParseFlags([]string{"--lb-s", "hi", "--lb-i", "7", "--lb-b", "--lb-plain", "p"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	out := &bcValueBucketArgs{}
	val := reflect.ValueOf(out).Elem()
	if err := bindFlags(cmd, val, specs); err != nil {
		t.Fatalf("bindFlags: %v", err)
	}
	if err := bindBuckets(cmd, val, specs); err != nil {
		t.Fatalf("bindBuckets: %v", err)
	}
	if out.Sel.S == nil || *out.Sel.S != "hi" {
		t.Errorf("Sel.S = %v, want hi", out.Sel.S)
	}
	if out.Sel.I == nil || *out.Sel.I != 7 {
		t.Errorf("Sel.I = %v, want 7", out.Sel.I)
	}
	if out.Sel.B == nil || *out.Sel.B != true {
		t.Errorf("Sel.B = %v, want true", out.Sel.B)
	}
	if out.Sel.Plain != "p" {
		t.Errorf("Sel.Plain = %q, want p", out.Sel.Plain)
	}
}

type bcPtrBucketArgs struct {
	Sel *bcLeafBucket
}

func TestBindBuckets_PointerBucketAllocates(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&bcPtrBucketArgs{}))
	_ = registerFlags(cmd, specs)
	if err := cmd.ParseFlags([]string{"--lb-s", "x"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	out := &bcPtrBucketArgs{}
	val := reflect.ValueOf(out).Elem()
	if err := bindBuckets(cmd, val, specs); err != nil {
		t.Fatalf("bindBuckets: %v", err)
	}
	if out.Sel == nil {
		t.Fatal("Sel pointer was not allocated")
	}
	if out.Sel.S == nil || *out.Sel.S != "x" {
		t.Errorf("Sel.S = %v, want x", out.Sel.S)
	}
}

// --- runNormalize / asString --------------------------------------------------

type bcNormField string

func (n bcNormField) Normalize(_ context.Context, raw string) (bcNormField, []string, error) {
	if raw == "boom" {
		return "", nil, errors.New("normalize failed")
	}
	return bcNormField("c:" + raw), []string{"hint: " + raw}, nil
}

type bcNormArgs struct {
	Token    bcNormField  `flag:"token"`
	TokenPtr *bcNormField `flag:"token-ptr"`
}

func TestRunNormalize_CanonicalizesAndEmitsHints(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{Use: "test"}
	cmd.SetErr(&buf)
	rt := &RuntimeContext{Cmd: cmd}

	ptr := bcNormField("xy")
	args := &bcNormArgs{Token: "raw", TokenPtr: &ptr}
	specs, _ := walkArgs(reflect.TypeOf(args))

	if err := runNormalize(context.Background(), rt, reflect.ValueOf(args).Elem(), specs); err != nil {
		t.Fatalf("runNormalize: %v", err)
	}
	if args.Token != "c:raw" {
		t.Errorf("Token = %q, want c:raw", args.Token)
	}
	if got := buf.String(); got == "" || !bytes.Contains([]byte(got), []byte("hint: raw")) {
		t.Errorf("stderr = %q, want it to contain the normalize hint", got)
	}
}

type bcBadNormArgs struct {
	Token bcNormField `flag:"token"`
}

func TestRunNormalize_PropagatesError(t *testing.T) {
	args := &bcBadNormArgs{Token: "boom"}
	specs, _ := walkArgs(reflect.TypeOf(args))
	err := runNormalize(context.Background(), nil, reflect.ValueOf(args).Elem(), specs)
	if err == nil {
		t.Fatal("expected error from Normalize")
	}
}

// --- checkGroup (via runFrameworkRules) ---------------------------------------

type bcGroupBody struct {
	A string `flag:"g-a"`
	B string `flag:"g-b"`
	C string `flag:"g-c" default:"x"` // default means never "missing"
}

type bcGroupArgs struct {
	Grp bcGroupBody
}

func TestCheckGroup_IncompleteReportsMissing(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&bcGroupArgs{}))
	_ = registerFlags(cmd, specs)
	// Only --g-a set: B is missing (no default), C has a default so it is fine.
	_ = cmd.ParseFlags([]string{"--g-a", "v"})
	out := &bcGroupArgs{}
	err := runFrameworkRules(cmd, reflect.ValueOf(out).Elem(), specs)
	if err == nil {
		t.Fatal("expected group_incomplete error")
	}
	ve := mustValidationError(t, err)
	if ve.Subtype != errs.SubtypeShortcutGroupIncomplete {
		t.Errorf("Subtype = %q, want group_incomplete", ve.Subtype)
	}
}

func TestCheckGroup_CompleteAndUntouched(t *testing.T) {
	specs, _ := walkArgs(reflect.TypeOf(&bcGroupArgs{}))

	// Complete: A and B provided.
	cmd1 := &cobra.Command{Use: "t1"}
	_ = registerFlags(cmd1, specs)
	_ = cmd1.ParseFlags([]string{"--g-a", "1", "--g-b", "2"})
	if err := runFrameworkRules(cmd1, reflect.ValueOf(&bcGroupArgs{}).Elem(), specs); err != nil {
		t.Errorf("complete group should pass, got %v", err)
	}

	// Untouched: nothing set → group rule does not fire.
	cmd2 := &cobra.Command{Use: "t2"}
	_ = registerFlags(cmd2, specs)
	_ = cmd2.ParseFlags(nil)
	if err := runFrameworkRules(cmd2, reflect.ValueOf(&bcGroupArgs{}).Elem(), specs); err != nil {
		t.Errorf("untouched group should pass, got %v", err)
	}
}

// --- checkEnumAndRequired (via runFrameworkRules) -----------------------------

type bcEnumArgs struct {
	Mode string `flag:"mode" enum:"a,b,c"`
}

func TestCheckEnum(t *testing.T) {
	specs, _ := walkArgs(reflect.TypeOf(&bcEnumArgs{}))

	// Invalid value.
	cmd1 := &cobra.Command{Use: "t1"}
	_ = registerFlags(cmd1, specs)
	_ = cmd1.ParseFlags([]string{"--mode", "z"})
	err := runFrameworkRules(cmd1, reflect.ValueOf(&bcEnumArgs{}).Elem(), specs)
	if err == nil {
		t.Fatal("expected invalid_argument error for bad enum value")
	}
	if ve := mustValidationError(t, err); ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want invalid_argument", ve.Subtype)
	}

	// Valid value.
	cmd2 := &cobra.Command{Use: "t2"}
	_ = registerFlags(cmd2, specs)
	_ = cmd2.ParseFlags([]string{"--mode", "b"})
	if err := runFrameworkRules(cmd2, reflect.ValueOf(&bcEnumArgs{}).Elem(), specs); err != nil {
		t.Errorf("valid enum value should pass, got %v", err)
	}

	// Empty (no default) → enum check skipped.
	cmd3 := &cobra.Command{Use: "t3"}
	_ = registerFlags(cmd3, specs)
	_ = cmd3.ParseFlags(nil)
	if err := runFrameworkRules(cmd3, reflect.ValueOf(&bcEnumArgs{}).Elem(), specs); err != nil {
		t.Errorf("empty enum value should pass, got %v", err)
	}
}

// --- runValidateValue recursion into a group sub-struct -----------------------

type bcValGroup struct {
	ID dummyValidatable `flag:"vg-id"`
}

type bcValGroupArgs struct {
	Grp bcValGroup
}

func TestRunValidateValue_RecursesIntoGroup(t *testing.T) {
	args := &bcValGroupArgs{}
	specs, _ := walkArgs(reflect.TypeOf(args))
	rt := &RuntimeContext{}
	if err := runValidateValue(rt, reflect.ValueOf(args).Elem(), specs); err != nil {
		t.Errorf("runValidateValue into group: %v", err)
	}
}
