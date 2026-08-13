// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package auth

import (
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/identitydiag"
)

type editionStatusState struct{}

func inspectEditionStatus(*cmdutil.Factory) (editionStatusState, error) {
	return editionStatusState{}, nil
}

func applyEditionStatus(map[string]interface{}, identitydiag.Result, editionStatusState) bool {
	return false
}
