// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package auth

import (
	"context"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/identitydiag"
)

type editionStatusState struct {
	provider string
	variant  string
}

func inspectEditionStatus(f *cmdutil.Factory) (editionStatusState, error) {
	if f == nil || f.Credential == nil {
		return editionStatusState{}, nil
	}
	source, err := f.Credential.InspectSource(context.Background())
	if err != nil {
		return editionStatusState{}, err
	}
	if source == nil || !source.Managed {
		return editionStatusState{}, nil
	}
	state := editionStatusState{provider: source.Name}
	description := f.RuntimeDescription()
	if description.Managed {
		state.variant = description.Variant
	}
	return state, nil
}

func applyEditionStatus(result map[string]interface{}, diagnostics identitydiag.Result, state editionStatusState) bool {
	if state.provider == "" {
		return false
	}
	result["source"] = "external"
	result["credentialProvider"] = state.provider
	if state.variant != "" {
		result["externalCredentialMode"] = state.variant
	}
	switch {
	case !diagnostics.User.Available && diagnostics.Bot.Available:
		result["note"] = "User identity is " + identitydiag.StatusMessage(diagnostics.User.Status) +
			"; bot identity is ready. Update authorization through external credential provider " + state.provider + "."
	case diagnostics.User.Status == identitydiag.StatusNeedsRefresh:
		result["note"] = "User identity needs refresh. Check external credential provider " + state.provider + "."
	case !diagnostics.User.Available && !diagnostics.Bot.Available:
		result["note"] = "No usable identity is available. Check external credential provider " + state.provider + "."
	}
	return true
}
