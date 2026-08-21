// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import (
	"fmt"
	"maps"
)

const IdentityDefaultedNoticeKey = "identity_defaulted"

// IdentityDefaultedMessage explains both the observed choice and why callers
// should make it explicit when reproducibility matters.
func IdentityDefaultedMessage(identity string) string {
	return fmt.Sprintf("--as was omitted; this IM write used %s. Pass --as explicitly for reproducible behavior.", identity)
}

// WithIdentityDefaultedNotice returns a copy of base with the command-scoped
// notice added. The copy prevents an invocation-specific fact from leaking
// into the process-wide update/skills notice map.
func WithIdentityDefaultedNotice(base map[string]interface{}, identity string) map[string]interface{} {
	notice := maps.Clone(base)
	if notice == nil {
		notice = make(map[string]interface{}, 1)
	}
	notice[IdentityDefaultedNoticeKey] = map[string]interface{}{
		"resolved": identity,
		"message":  IdentityDefaultedMessage(identity),
	}
	return notice
}
