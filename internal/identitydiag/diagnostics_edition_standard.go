// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package identitydiag

import (
	"context"
	"errors"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func diagnoseEditionSource(
	ctx context.Context,
	f *cmdutil.Factory,
	cfg *core.CliConfig,
	verify bool,
) (Result, bool) {
	if f == nil || f.Credential == nil {
		return Result{}, false
	}
	source, err := f.Credential.InspectSource(ctx)
	if err != nil {
		var blockErr *extcred.BlockError
		if !errors.As(err, &blockErr) {
			return Result{}, false
		}
		provider := blockErr.Provider
		if source != nil && source.Name != "" {
			provider = source.Name
		}
		if provider == "" {
			provider = "external"
		}
		return diagnoseExternal(ctx, f, cfg, provider, verify), true
	}
	if source == nil || !source.Managed {
		return Result{}, false
	}
	return diagnoseExternal(ctx, f, cfg, source.Name, verify), true
}

func withEditionIdentity(ctx context.Context, _ core.Identity) context.Context {
	return ctx
}
