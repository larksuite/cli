// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdupdate

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/distribution"
	"github.com/larksuite/cli/internal/output"
)

func runManifestUpdate(ctx context.Context, opts *UpdateOptions, manifestURL string) error {
	streams := opts.Factory.IOStreams
	current := currentVersion()
	manifest, err := distribution.FetchManifest(ctx, manifestURL)
	if err != nil {
		return reportDistributionError(opts, err)
	}
	target := manifest.Version
	if opts.Check {
		return reportManifestStatus(opts, current, target, true)
	}
	if !opts.Force && target == current {
		return reportManifestStatus(opts, current, target, false)
	}
	if !opts.JSON {
		fmt.Fprintf(streams.ErrOut, "Updating lark-cli %s %s %s from the configured distribution ...\n", current, symArrow(), target)
	}
	if err := distribution.Install(ctx, manifest, distribution.InstallOptions{}); err != nil {
		return reportDistributionError(opts, err)
	}
	if opts.JSON {
		output.PrintJson(streams.Out, map[string]interface{}{
			"ok": true, "source": "manifest",
			"previous_version": current, "current_version": target, "target_version": target,
			"action": "updated", "skills_action": "synced",
			"message": fmt.Sprintf("lark-cli updated from %s to %s", current, target),
		})
		return nil
	}
	fmt.Fprintf(streams.ErrOut, "\n%s Successfully updated lark-cli and Skills from %s to %s\n", symOK(), current, target)
	return nil
}

func reportManifestStatus(opts *UpdateOptions, current, target string, check bool) error {
	streams := opts.Factory.IOStreams
	action := "already_up_to_date"
	message := fmt.Sprintf("lark-cli %s matches the configured target", current)
	if current != target {
		action = "update_available"
		message = fmt.Sprintf("lark-cli %s %s configured target %s", current, symArrow(), target)
	}
	if opts.JSON {
		result := map[string]interface{}{
			"ok": true, "source": "manifest",
			"previous_version": current, "current_version": current, "target_version": target,
			"action": action, "message": message,
		}
		if check {
			result["auto_update"] = true
		}
		output.PrintJson(streams.Out, result)
		return nil
	}
	if current == target {
		fmt.Fprintf(streams.ErrOut, "%s %s\n", symOK(), message)
	} else {
		fmt.Fprintf(streams.ErrOut, "Configured target: %s %s %s\n\nRun `lark-cli update` to install.\n", current, symArrow(), target)
	}
	return nil
}

func reportDistributionError(opts *UpdateOptions, typed errs.TypedError) error {
	errType := "update_error"
	if problem, ok := errs.ProblemOf(typed); ok && problem.Category == errs.CategoryNetwork {
		errType = "network"
	}
	return reportError(opts, opts.Factory.IOStreams, errType, typed)
}
