// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	_ "github.com/larksuite/cli/internal/vfs/localfileio"
)

// newTypedInputRuntime registers the given specs on a fresh cobra command,
// parses argv, and returns a RuntimeContext wired with a fake stdin — the
// typed-binder analogue of newTestRuntimeWithStdin in runner_input_test.go.
func newTypedInputRuntime(t *testing.T, specs []fieldSpec, argv []string, stdin string) *RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	if err := registerFlags(cmd, specs); err != nil {
		t.Fatalf("registerFlags: %v", err)
	}
	if err := cmd.ParseFlags(argv); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	return &RuntimeContext{
		Cmd: cmd,
		Factory: &cmdutil.Factory{
			IOStreams: &cmdutil.IOStreams{In: strings.NewReader(stdin)},
		},
	}
}

// --- @file / stdin on typed shortcuts -------------------------------------

type fileInputArgs struct {
	Content string `flag:"content" input:"file,stdin"`
}

func TestResolveTypedInputs_File(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	body := "## Title\n\nbody from a file\n"
	if err := os.WriteFile("body.md", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	specs, err := walkArgs(reflect.TypeOf(&fileInputArgs{}))
	if err != nil {
		t.Fatalf("walkArgs: %v", err)
	}
	rt := newTypedInputRuntime(t, specs, []string{"--content", "@body.md"}, "")
	if err := resolveTypedInputs(rt, specs); err != nil {
		t.Fatalf("resolveTypedInputs: %v", err)
	}
	out := &fileInputArgs{}
	if err := bindFlags(rt.Cmd, reflect.ValueOf(out).Elem(), specs); err != nil {
		t.Fatalf("bindFlags: %v", err)
	}
	if out.Content != body {
		t.Errorf("Content = %q, want file body %q", out.Content, body)
	}
}

func TestResolveTypedInputs_Stdin(t *testing.T) {
	specs, _ := walkArgs(reflect.TypeOf(&fileInputArgs{}))
	rt := newTypedInputRuntime(t, specs, []string{"--content", "-"}, "piped stdin body")
	if err := resolveTypedInputs(rt, specs); err != nil {
		t.Fatalf("resolveTypedInputs: %v", err)
	}
	out := &fileInputArgs{}
	_ = bindFlags(rt.Cmd, reflect.ValueOf(out).Elem(), specs)
	if out.Content != "piped stdin body" {
		t.Errorf("Content = %q, want stdin body", out.Content)
	}
}

func TestResolveTypedInputs_PlainValueUnchanged(t *testing.T) {
	specs, _ := walkArgs(reflect.TypeOf(&fileInputArgs{}))
	rt := newTypedInputRuntime(t, specs, []string{"--content", "literal text"}, "")
	if err := resolveTypedInputs(rt, specs); err != nil {
		t.Fatalf("resolveTypedInputs: %v", err)
	}
	out := &fileInputArgs{}
	_ = bindFlags(rt.Cmd, reflect.ValueOf(out).Elem(), specs)
	if out.Content != "literal text" {
		t.Errorf("Content = %q, want unchanged literal", out.Content)
	}
}

// A OneOf variant flag that declares @file/stdin must resolve too — the binder
// recurses into buckets because cobra flags are flat regardless of nesting.
type nestedInputVariant struct {
	Body *string `flag:"body" input:"file,stdin"`
	Raw  *string `flag:"raw"`
}

func (nestedInputVariant) OneOf() {}

type nestedInputArgs struct {
	Content nestedInputVariant
}

func TestResolveTypedInputs_NestedInOneOf(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	body := "nested variant body\n"
	if err := os.WriteFile("v.md", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	specs, err := walkArgs(reflect.TypeOf(&nestedInputArgs{}))
	if err != nil {
		t.Fatalf("walkArgs: %v", err)
	}
	rt := newTypedInputRuntime(t, specs, []string{"--body", "@v.md"}, "")
	if err := resolveTypedInputs(rt, specs); err != nil {
		t.Fatalf("resolveTypedInputs: %v", err)
	}
	out := &nestedInputArgs{}
	if err := bindBuckets(rt.Cmd, reflect.ValueOf(out).Elem(), specs); err != nil {
		t.Fatalf("bindBuckets: %v", err)
	}
	if out.Content.Body == nil {
		t.Fatal("Content.Body is nil — variant not bound")
	}
	if *out.Content.Body != body {
		t.Errorf("Content.Body = %q, want file body %q", *out.Content.Body, body)
	}
}

// --- Mount-time validation: enum / input only on string leaves ------------

type enumOnIntArgs struct {
	Level int `flag:"level" enum:"1,2,3"`
}

func TestWalkArgs_EnumOnNonStringErrors(t *testing.T) {
	_, err := walkArgs(reflect.TypeOf(&enumOnIntArgs{}))
	if err == nil {
		t.Fatal("expected error for enum on int field")
	}
	if !strings.Contains(err.Error(), "enum tag is only supported on string") {
		t.Errorf("unexpected error: %v", err)
	}
}

type inputOnBoolArgs struct {
	Flag bool `flag:"flag" input:"file"`
}

func TestWalkArgs_InputOnNonStringErrors(t *testing.T) {
	_, err := walkArgs(reflect.TypeOf(&inputOnBoolArgs{}))
	if err == nil {
		t.Fatal("expected error for input on bool field")
	}
	if !strings.Contains(err.Error(), "input tag is only supported on string") {
		t.Errorf("unexpected error: %v", err)
	}
}

type unknownInputSrcArgs struct {
	Content string `flag:"content" input:"bogus"`
}

func TestWalkArgs_UnknownInputSource(t *testing.T) {
	_, err := walkArgs(reflect.TypeOf(&unknownInputSrcArgs{}))
	if err == nil {
		t.Fatal("expected error for unknown input source")
	}
	if !strings.Contains(err.Error(), "unknown input source") {
		t.Errorf("unexpected error: %v", err)
	}
}

// A string-alias enum field (e.g. an argstype primitive) must be accepted.
type enumOnStringArgs struct {
	Priority string `flag:"priority" enum:"low,normal,high"`
}

func TestWalkArgs_EnumOnStringOK(t *testing.T) {
	specs, err := walkArgs(reflect.TypeOf(&enumOnStringArgs{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 || len(specs[0].EnumValues) != 3 {
		t.Errorf("enum not parsed: %+v", specs)
	}
}

// --- enum shell completion + help candidate rendering ---------------------

func TestRegisterLeaf_EnumCompletion(t *testing.T) {
	prev := cmdutil.FlagCompletionsEnabled()
	cmdutil.SetFlagCompletionsEnabled(true)
	defer cmdutil.SetFlagCompletionsEnabled(prev)

	specs, _ := walkArgs(reflect.TypeOf(&enumOnStringArgs{}))
	cmd := &cobra.Command{Use: "test"}
	if err := registerFlags(cmd, specs); err != nil {
		t.Fatalf("registerFlags: %v", err)
	}
	fn, ok := cmd.GetFlagCompletionFunc("priority")
	if !ok || fn == nil {
		t.Fatal("expected a completion func registered for --priority")
	}
	vals, _ := fn(cmd, nil, "")
	want := map[string]bool{"low": true, "normal": true, "high": true}
	if len(vals) != 3 {
		t.Fatalf("completion candidates = %v, want low/normal/high", vals)
	}
	for _, v := range vals {
		if !want[v] {
			t.Errorf("unexpected completion candidate %q", v)
		}
	}
}

func TestFormatLeafLine_EnumCandidates(t *testing.T) {
	s := fieldSpec{FlagName: "priority", Description: "the priority", EnumValues: []string{"low", "normal", "high"}}
	line := formatLeafLine("  ", s)
	if !strings.Contains(line, "(one of: low|normal|high)") {
		t.Errorf("help line missing enum candidates: %q", line)
	}
}

func TestFormatLeafLine_EnumAndDefault(t *testing.T) {
	s := fieldSpec{FlagName: "priority", Description: "the priority", EnumValues: []string{"low", "high"}, DefaultValue: "low"}
	line := formatLeafLine("  ", s)
	if !strings.Contains(line, "(one of: low|high)") || !strings.Contains(line, `(default "low")`) {
		t.Errorf("help line missing enum or default: %q", line)
	}
}
