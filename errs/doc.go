// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package errs is the public error-contract surface for lark-cli.
//
// It defines a closed taxonomy (9 Categories) and a small set of typed
// errors that embed Problem — an RFC 7807-aligned shared shape. External
// consumers (AI agents, shell scripts, integrating SDKs) read structured
// fields instead of regex-parsing free-string error messages.
//
// # The Problem shape
//
// Every typed error embeds Problem so the JSON wire shape (`type`,
// `subtype`, `code`, `message`, `hint`, `log_id`, `retryable`) is uniform
// across categories. Typed extensions (PermissionError.MissingScopes,
// SecurityPolicyError.ChallengeURL, etc.) appear at the top level of the
// envelope alongside the shared fields, not nested under a `detail` key.
//
// # Working with typed errors
//
// Use ProblemOf to read shared fields polymorphically:
//
//	if p, ok := errs.ProblemOf(err); ok {
//	    log.Printf("category=%s subtype=%s retryable=%t", p.Category, p.Subtype, p.Retryable)
//	}
//
// Use the IsXxx predicates or stdlib errors.As to branch on concrete type:
//
//	if errs.IsPermission(err) {
//	    var pe *errs.PermissionError
//	    _ = errors.As(err, &pe)
//	    fmt.Println("missing scopes:", pe.MissingScopes)
//	}
//
// Use WrapInternal at boundaries to lift any non-typed error to
// *InternalError; typed errors pass through unchanged.
package errs
