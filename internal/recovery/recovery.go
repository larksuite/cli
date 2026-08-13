// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package recovery

import (
	"strings"

	"github.com/larksuite/cli/errs"
)

// Target identifies the kind of recovery action a hint points to.
type Target string

const (
	// TargetSchema points users to the generated request schema.
	TargetSchema Target = "schema"
)

// Hint is a rendered, user-facing recovery hint.
type Hint string

// Text returns a plain recovery hint fragment.
func Text(s string) Hint {
	return Hint(strings.TrimSpace(s))
}

// Command returns a command recovery hint fragment.
func Command(_ Target, s string) Hint {
	return Text(s)
}

// Join combines non-empty hint fragments with sep.
func Join(sep string, hints ...Hint) Hint {
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		if s := strings.TrimSpace(string(hint)); s != "" {
			parts = append(parts, s)
		}
	}
	return Hint(strings.Join(parts, sep))
}

// Attach applies hint to typed errors that support WithHint.
func Attach(err error, hint Hint) error {
	if err == nil {
		return nil
	}
	s := strings.TrimSpace(string(hint))
	if s == "" {
		return err
	}
	switch e := err.(type) {
	case *errs.ValidationError:
		return e.WithHint("%s", s)
	case *errs.AuthenticationError:
		return e.WithHint("%s", s)
	case *errs.PermissionError:
		return e.WithHint("%s", s)
	case *errs.ConfigError:
		return e.WithHint("%s", s)
	case *errs.NetworkError:
		return e.WithHint("%s", s)
	case *errs.APIError:
		return e.WithHint("%s", s)
	case *errs.SecurityPolicyError:
		return e.WithHint("%s", s)
	case *errs.ContentSafetyError:
		return e.WithHint("%s", s)
	case *errs.InternalError:
		return e.WithHint("%s", s)
	case *errs.ConfirmationRequiredError:
		return e.WithHint("%s", s)
	default:
		return err
	}
}
