// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

// Category is the top-level taxonomy axis. Wire JSON: "type".
type Category string

const (
	CategoryValidation     Category = "validation"
	CategoryAuthentication Category = "authentication"
	CategoryAuthorization  Category = "authorization"
	CategoryConfig         Category = "config"
	CategoryNetwork        Category = "network"
	CategoryAPI            Category = "api"
	CategoryPolicy         Category = "policy"
	CategoryInternal       Category = "internal"
	CategoryConfirmation   Category = "confirmation"
)
