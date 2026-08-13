// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package externalcredential

import (
	"github.com/larksuite/cli/errs"
)

func requireExtendedEdition() error {
	return extendedEditionRequired()
}

func extendedEditionRequired() error {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"system external credential configuration requires the lark-cli Extended edition").
		WithHint("install lark-cli Extended or ask the administrator to remove external-credential.json")
}

func validateTrustedSystemConfig(*Config) error { return nil }
