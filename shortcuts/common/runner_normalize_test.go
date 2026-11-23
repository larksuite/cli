// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestRunShortcutNormalizesAfterInputAndBeforeCanonicalValidation(t *testing.T) {
	var phases []string
	s := &Shortcut{
		Service:   "test",
		Command:   "test-shortcut",
		AuthTypes: []string{"bot"},
		Flags: []Flag{
			{Name: "canonical", Enum: []string{"normalized"}},
			{Name: "legacy", Input: []string{Stdin}},
		},
		Normalize: func(_ context.Context, flags *FlagContext) error {
			phases = append(phases, "normalize:"+flags.Str("legacy"))
			return flags.SetCanonical("canonical", "normalized")
		},
		Validate: func(_ context.Context, runtime *RuntimeContext) error {
			phases = append(phases, "validate:"+runtime.Str("canonical"))
			return nil
		},
		Execute: func(_ context.Context, runtime *RuntimeContext) error {
			phases = append(phases, "execute:"+runtime.Str("canonical"))
			return nil
		},
	}
	factory := newTestFactory()
	factory.IOStreams.In = strings.NewReader("resolved-input")
	cmd := newTestShortcutCmd(s, factory)
	if err := cmd.Flags().Set("legacy", "-"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("as", "bot"); err != nil {
		t.Fatal(err)
	}

	if err := runShortcut(cmd, factory, s, true); err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}
	want := "normalize:resolved-input,validate:normalized,execute:normalized"
	if got := strings.Join(phases, ","); got != want {
		t.Fatalf("phases = %q, want %q", got, want)
	}
}

func TestRunShortcutNormalizeFailureStopsCanonicalConsumers(t *testing.T) {
	s := &Shortcut{
		Service:   "test",
		Command:   "test-shortcut",
		AuthTypes: []string{"bot"},
		Normalize: func(context.Context, *FlagContext) error {
			return ValidationErrorf("legacy compatibility failed").WithParam("--legacy")
		},
		Validate: func(context.Context, *RuntimeContext) error {
			t.Fatal("Validate ran after Normalize failed")
			return nil
		},
		Execute: func(context.Context, *RuntimeContext) error {
			t.Fatal("Execute ran after Normalize failed")
			return nil
		},
	}
	factory := newTestFactory()
	cmd := newTestShortcutCmd(s, factory)
	if err := cmd.Flags().Set("as", "bot"); err != nil {
		t.Fatal(err)
	}
	if err := runShortcut(cmd, factory, s, true); err == nil {
		t.Fatal("runShortcut() error = nil")
	}
}

func TestSetCanonicalFromClassifiesPFlagConversionFailure(t *testing.T) {
	s := &Shortcut{
		Service:   "test",
		Command:   "test-shortcut",
		AuthTypes: []string{"bot"},
		Flags: []Flag{
			{Name: "canonical", Type: "int"},
			{Name: "legacy"},
		},
		Normalize: func(_ context.Context, flags *FlagContext) error {
			return flags.SetCanonicalFrom("legacy", "canonical", flags.Str("legacy"))
		},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}
	factory := newTestFactory()
	cmd := newTestShortcutCmd(s, factory)
	if err := cmd.Flags().Set("legacy", "not-an-int"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("as", "bot"); err != nil {
		t.Fatal(err)
	}

	err := runShortcut(cmd, factory, s, true)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want typed validation error", err, err)
	}
	if validationErr.Param != "--legacy" {
		t.Fatalf("param = %q, want --legacy", validationErr.Param)
	}
	if errors.Unwrap(validationErr) == nil {
		t.Fatal("pflag conversion cause was not preserved")
	}
}

func TestMountedShortcutNormalizeDoesNotExpandCobraPreRun(t *testing.T) {
	normalizeCalled := false
	shortcut := Shortcut{
		Service: "test", Command: "+normalize-required", Description: "x",
		Flags: []Flag{
			{Name: "canonical", Required: true},
			{Name: "legacy", Hidden: true},
		},
		Normalize: func(_ context.Context, flags *FlagContext) error {
			normalizeCalled = true
			if !flags.Changed("legacy") || flags.Changed("canonical") {
				return nil
			}
			return flags.SetCanonicalFrom("legacy", "canonical", flags.Str("legacy"))
		},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}
	cmd := mountTestShortcut(t, shortcut)
	if err := cmd.ParseFlags([]string{"--legacy", "accepted"}); err != nil {
		t.Fatal(err)
	}
	if cmd.PreRunE != nil || cmd.PreRun != nil {
		t.Fatal("Normalize must not install or take over Cobra PreRun hooks")
	}
	if err := cmd.ValidateRequiredFlags(); err == nil {
		t.Fatal("a business Normalize hook must not satisfy Cobra Required")
	}
	if normalizeCalled {
		t.Fatal("Normalize ran before Cobra Required validation")
	}
}

func TestChainNormalizersPreservesDeclarationOrderAndStopsOnError(t *testing.T) {
	var phases []string
	stop := ValidationErrorf("stop")
	chain := ChainNormalizers(
		func(context.Context, *FlagContext) error {
			phases = append(phases, "first")
			return nil
		},
		nil,
		func(context.Context, *FlagContext) error {
			phases = append(phases, "second")
			return stop
		},
		func(context.Context, *FlagContext) error {
			t.Fatal("normalizer ran after an error")
			return nil
		},
	)
	if err := chain(context.Background(), nil); err != stop {
		t.Fatalf("error = %v, want stop", err)
	}
	if got := strings.Join(phases, ","); got != "first,second" {
		t.Fatalf("phases = %q", got)
	}
}
