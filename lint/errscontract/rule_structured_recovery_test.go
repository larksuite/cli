// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"strings"
	"testing"
)

func TestStructuredRecoveryRejectsOpaqueAuthLoginInTypedError(t *testing.T) {
	src := `package demo

import "github.com/larksuite/cli/errs"

func run() error {
	return errs.NewAuthenticationError(errs.SubtypeTokenMissing, "not logged in").
		WithHint("run lark-cli auth login --scope demo:read")
}
`
	got := CheckStructuredRecovery("shortcuts/demo/demo.go", src)
	if len(got) != 1 || got[0].Rule != "structured_recovery" || got[0].Action != ActionReject {
		t.Fatalf("violations = %#v, want one structured_recovery rejection", got)
	}
}

func TestStructuredRecoveryRejectsOpaqueAuthLoginInMessageAssignment(t *testing.T) {
	src := `package demo

import "github.com/larksuite/cli/errs"

func enrich(err *errs.PermissionError) error {
	err.Message = "run lark-cli auth login"
	return err
}
`
	got := CheckStructuredRecovery("shortcuts/demo/demo.go", src)
	if len(got) != 1 {
		t.Fatalf("violations = %#v, want one", got)
	}
}

func TestStructuredRecoveryRejectsOpaqueFrameworkCommands(t *testing.T) {
	for _, command := range []string{"lark-cli config bind", "lark-cli profile add"} {
		t.Run(command, func(t *testing.T) {
			src := `package demo

import "github.com/larksuite/cli/errs"

func run() error {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, "not ready").
		WithHint("run ` + command + ` first")
			}
`
			got := CheckStructuredRecovery("cmd/demo/demo.go", src)
			if len(got) != 1 || !strings.Contains(got[0].Message, command) {
				t.Fatalf("violations = %#v, want one rejection naming %q", got, command)
			}
		})
	}
}

func TestStructuredRecoveryAcceptsSemanticRecovery(t *testing.T) {
	src := `package demo

import (
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/recovery"
)

func run() error {
	return recovery.Attach(
		errs.NewAuthenticationError(errs.SubtypeTokenMissing, "not logged in"),
		recovery.UserAuthorization("demo:read"),
	)
}
`
	if got := CheckStructuredRecovery("shortcuts/demo/demo.go", src); len(got) != 0 {
		t.Fatalf("semantic recovery rejected: %#v", got)
	}
}
