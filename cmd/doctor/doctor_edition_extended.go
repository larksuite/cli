// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package doctor

import (
	"errors"
	"fmt"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/identitydiag"
)

func runEditionDoctor(opts *DoctorOptions, checks []checkResult) (bool, error) {
	f := opts.Factory
	if f == nil || f.Credential == nil {
		return false, nil
	}
	source, err := f.Credential.InspectSource(opts.Ctx)
	if err != nil {
		checks = append(checks, fail("credential_source", err.Error(), editionDiagnosticErrorHint(err)))
		return true, finishDoctor(f, checks)
	}
	if source == nil || !source.Managed {
		return false, nil
	}

	provider := source.Name
	cfg, err := f.Config()
	if err != nil {
		checks = append(checks,
			fail("credential_source", err.Error(), editionDiagnosticErrorHint(err)),
			skip("config_file", fmt.Sprintf("local credentials are not used; source is %s", provider)),
		)
		return true, finishDoctor(f, checks)
	}
	checks = append(checks, pass("credential_source",
		fmt.Sprintf("credentials provided by %s (app %s; token not verified by this check)", provider, cfg.AppID)))

	description := f.RuntimeDescription()
	if description.Managed {
		checks = append(checks, pass("config_file", "config.json found (system external credential mode)"))
	} else {
		checks = append(checks, skip("config_file",
			fmt.Sprintf("local config not used; credentials provided by %s", provider)))
	}
	checks = append(checks, pass("app_resolved", fmt.Sprintf("app: %s (%s)", cfg.AppID, cfg.Brand)))
	checks = appendProfileSelectorCheck(checks, f, cfg)

	diagnostics := identitydiag.Diagnose(opts.Ctx, f, cfg, !opts.Offline)
	checks = append(checks,
		identityCheck("bot_identity", diagnostics.Bot),
		identityCheck("user_identity", diagnostics.User),
	)
	if diagnostics.Bot.Available || diagnostics.User.Available {
		checks = append(checks, pass("identity_ready", "at least one identity is available"))
	} else {
		checks = append(checks, fail("identity_ready", "no usable bot or user identity is available", ""))
	}

	if description.ProxiesRequests {
		checks = append(checks, editionProxyNetworkCheck(opts, description.DataPlaneEndpoint, diagnostics))
	} else {
		checks = append(checks, networkChecks(opts.Ctx, opts, core.ResolveEndpoints(cfg.Brand))...)
	}
	return true, finishDoctor(f, checks)
}

func editionDiagnosticErrorHint(err error) string {
	var blockErr *extcred.BlockError
	if errors.As(err, &blockErr) {
		return blockErr.Reason
	}
	var cfgErr *errs.ConfigError
	if errors.As(err, &cfgErr) {
		return cfgErr.Hint
	}
	return ""
}

func editionProxyNetworkCheck(opts *DoctorOptions, endpoint string, diagnostics identitydiag.Result) checkResult {
	if opts.Offline {
		return skip("endpoint_external_platform", "skipped (--offline)")
	}
	verified := func(id identitydiag.Identity) bool { return id.Verified != nil && *id.Verified }
	if verified(diagnostics.User) || verified(diagnostics.Bot) {
		return pass("endpoint_external_platform", endpoint+" reachable through an authenticated API request")
	}
	return fail("endpoint_external_platform", endpoint+" could not complete an authenticated API request",
		"check the external credential program and platform logs")
}
