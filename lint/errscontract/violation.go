// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import "github.com/larksuite/cli/lint/lintapi"

// Re-export the shared types so existing rule code reads Action /
// Violation locally. The canonical declarations live in lintapi.
type (
	Action    = lintapi.Action
	Violation = lintapi.Violation
)

const (
	ActionReject  = lintapi.ActionReject
	ActionLabel   = lintapi.ActionLabel
	ActionWarning = lintapi.ActionWarning
)

// subtypeClassification is the package-internal verdict produced by the
// CheckDeclaredSubtype classifier for a single Subtype: expression. Empty
// action means "accept silently".
type subtypeClassification struct {
	rule, message, suggestion string
	action                    Action
}
