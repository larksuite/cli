// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package runtimebootstrap

import (
	"github.com/larksuite/cli/internal/externalcredential"
	"github.com/larksuite/cli/internal/runtimeplan"
)

// resolveEdition is the only production composition edge from the CLI
// bootstrap into the external credential product implementation.
func resolveEdition(profileOverride string) *Result {
	selection, err := externalcredential.SelectProfile(profileOverride)
	result := &Result{Plan: runtimeplan.Default()}
	metadata := runtimeplan.MetadataRemoteAllowed
	if selection != nil {
		result.ProfileConfig = selection.Config
		if selection.DisableRemoteMeta {
			metadata = runtimeplan.MetadataEmbeddedOnly
		}
		if selection.Plan != nil {
			result.Plan = selection.Plan
		}
	}
	if err != nil {
		result.Plan = runtimeplan.Failed(err, metadata)
	}
	return result
}
