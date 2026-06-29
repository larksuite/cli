// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package shortcuts

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func newFactoryWithBrand(brand core.LarkBrand) *cmdutil.Factory {
	return &cmdutil.Factory{
		Config: func() (*core.CliConfig, error) {
			return &core.CliConfig{Brand: brand}, nil
		},
	}
}

func findChild(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func TestBrandGuard_AppsStaysRegisteredOnLark(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, newFactoryWithBrand(core.BrandLark))

	apps := findChild(program, "apps")
	if apps == nil {
		t.Fatal("apps service command should be registered on Lark brand (so users see a clear brand error, not 'unknown command')")
	}
	if !apps.Hidden {
		t.Error("apps service command should be Hidden on Lark brand")
	}
	if len(apps.Commands()) == 0 {
		t.Error("apps subcommands should still be mounted (so children also hit the brand-restriction stub)")
	}
	for _, child := range apps.Commands() {
		if !child.Hidden {
			t.Errorf("apps child %q should be Hidden on Lark brand", child.Name())
		}
	}
}

func TestBrandGuard_AppsExecuteReturnsBrandError(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, newFactoryWithBrand(core.BrandLark))

	apps := findChild(program, "apps")
	if apps == nil {
		t.Fatal("apps should be registered")
	}
	create := findChild(apps, "+create")
	if create == nil {
		t.Fatal("apps +create should be registered")
	}

	err := create.RunE(create, []string{"--name", "x"})
	if err == nil {
		t.Fatal("expected brand-restriction error, got nil")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if validationErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("expected subtype %q, got %q", errs.SubtypeFailedPrecondition, validationErr.Subtype)
	}
	if !strings.Contains(validationErr.Error(), "apps") || !strings.Contains(validationErr.Error(), "lark") {
		t.Errorf("expected error to mention apps + lark, got: %s", validationErr.Error())
	}
}

func TestBrandGuard_AppsExecutableOnFeishu(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, newFactoryWithBrand(core.BrandFeishu))

	apps := findChild(program, "apps")
	if apps == nil {
		t.Fatal("apps should be registered on Feishu brand")
	}
	if apps.Hidden {
		t.Error("apps should NOT be Hidden on Feishu brand")
	}
	create := findChild(apps, "+create")
	if create == nil {
		t.Fatal("apps +create should be registered on Feishu brand")
	}
	if create.DisableFlagParsing {
		t.Error("apps +create should not have DisableFlagParsing on Feishu (the guard must not have run)")
	}
}

func TestBrandGuard_DispatchHitsStubViaCobra(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, newFactoryWithBrand(core.BrandLark))

	program.SetArgs([]string{"apps", "+create", "--name", "x", "--app-type", "HTML"})
	program.SetContext(context.Background())
	err := program.Execute()
	if err == nil {
		t.Fatal("expected error from dispatching apps +create on Lark brand")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError from cobra dispatch, got %T: %v", err, err)
	}
	if !strings.Contains(validationErr.Error(), "lark") {
		t.Errorf("dispatched error should mention lark brand, got: %s", validationErr.Error())
	}
}
