// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"errors"
	"strings"
)

// ErrInvalidRef is the sentinel error for a malformed agent_ref (wrapped into a
// validation error by the caller).
var ErrInvalidRef = errors.New("agent_ref must look like <provider>:<agent_id>")

// Ref is the identifier addressing a remote agent: <scheme>:<agent_id>, e.g. base:assistant.
type Ref struct {
	Scheme  string
	AgentID string
}

// ParseRef parses a ref string. On a malformed format it returns ErrInvalidRef
// (wrapped into a validation error by the caller).
func ParseRef(s string) (Ref, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(parts[1], ":") {
		return Ref{}, ErrInvalidRef
	}
	return Ref{Scheme: parts[0], AgentID: parts[1]}, nil
}
