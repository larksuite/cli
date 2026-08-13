// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package output

import "github.com/larksuite/cli/errs"

func setDefaultErrorOrigin(err error) error {
	if metadata, ok := errs.DiagnosticMetadataOf(err); ok && metadata.Origin != "" {
		return err
	}
	return errs.WithDiagnosticMetadata(err, errs.DiagnosticMetadata{Origin: "cli"})
}

func defaultErrorOrigin() string { return "cli" }
