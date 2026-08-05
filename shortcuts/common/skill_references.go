// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/meta"
)

// ResolveAffordanceSkillReferences returns the current command's related skill
// references after applying this build's skill overlay, remaps, and command
// presentation. It is the execution-time counterpart of help rendering: an
// error producer can offer the same version-matched guidance without copying
// canonical paths or publishing a `skills read` command that this distribution
// cannot execute.
func (ctx *RuntimeContext) ResolveAffordanceSkillReferences() []string {
	if ctx == nil || ctx.Cmd == nil || ctx.Factory == nil {
		return nil
	}
	service, methodID, ok := cmdmeta.AffordanceRef(ctx.Cmd)
	if !ok {
		return nil
	}
	raw, ok := affordance.For(service, methodID)
	if !ok {
		return nil
	}
	parsed, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		return nil
	}

	resolved := make([]string, 0, len(parsed.Skills))
	for _, canonical := range parsed.Skills {
		if ref, ok := ctx.Factory.ResolveSkillReference(canonical); ok {
			resolved = append(resolved, ref)
		}
	}
	return resolved
}
