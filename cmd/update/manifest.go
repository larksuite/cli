// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdupdate

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/distribution"
	"github.com/larksuite/cli/internal/distributioninstall"
	"github.com/larksuite/cli/internal/output"
)

func runManifestUpdate(ctx context.Context, opts *UpdateOptions, source distribution.Source) error {
	streams := opts.Factory.IOStreams
	current := currentVersion()
	manifest, err := distribution.FetchManifest(ctx, source)
	if err != nil {
		return reportDistributionError(opts, "failed to load distribution manifest", err)
	}
	target := manifest.Version
	if opts.Check {
		return reportManifestCheck(opts, current, target)
	}
	if !opts.Force && target == current {
		return reportManifestCurrent(opts, current)
	}
	if !opts.JSON {
		fmt.Fprintf(streams.ErrOut, "Updating lark-cli %s %s %s from the configured distribution ...\n", current, symArrow(), target)
	}
	prepared, err := distribution.PrepareUpdate(ctx, manifest)
	if err != nil {
		return reportDistributionError(opts, "failed to prepare distribution update", err)
	}
	defer prepared.Cleanup()
	if err := distributioninstall.InstallPrepared(prepared, distributioninstall.InstallOptions{}); err != nil {
		return reportError(opts, streams, "update_error",
			errs.NewInternalError(errs.SubtypeUnknown, "failed to install distribution update: %s", err).
				WithHint("Retry with `lark-cli update --force`.").WithCause(err))
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

func reportManifestCheck(opts *UpdateOptions, current, target string) error {
	streams := opts.Factory.IOStreams
	action := "already_up_to_date"
	message := fmt.Sprintf("lark-cli %s matches the configured target", current)
	if current != target {
		action = "update_available"
		message = fmt.Sprintf("lark-cli %s %s configured target %s", current, symArrow(), target)
	}
	if opts.JSON {
		output.PrintJson(streams.Out, map[string]interface{}{
			"ok": true, "source": "manifest",
			"previous_version": current, "current_version": current, "target_version": target,
			"action": action, "auto_update": true, "message": message,
		})
		return nil
	}
	if current == target {
		fmt.Fprintf(streams.ErrOut, "%s %s\n", symOK(), message)
	} else {
		fmt.Fprintf(streams.ErrOut, "Configured target: %s %s %s\n\nRun `lark-cli update` to install.\n", current, symArrow(), target)
	}
	return nil
}

func reportManifestCurrent(opts *UpdateOptions, current string) error {
	streams := opts.Factory.IOStreams
	if opts.JSON {
		output.PrintJson(streams.Out, map[string]interface{}{
			"ok": true, "source": "manifest",
			"previous_version": current, "current_version": current, "target_version": current,
			"action":  "already_up_to_date",
			"message": fmt.Sprintf("lark-cli %s matches the configured target", current),
		})
		return nil
	}
	fmt.Fprintf(streams.ErrOut, "%s lark-cli %s matches the configured target\n", symOK(), current)
	return nil
}

func reportDistributionError(opts *UpdateOptions, message string, err error) error {
	typed := classifyDistributionError(message, err)
	errType := "update_error"
	if problem, ok := errs.ProblemOf(typed); ok && problem.Category == errs.CategoryNetwork {
		errType = "network"
	}
	return reportError(opts, opts.Factory.IOStreams, errType, typed)
}

func classifyDistributionError(message string, err error) errs.TypedError {
	var typed errs.TypedError
	if errors.As(err, &typed) {
		return typed
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return errs.NewInternalError(errs.SubtypeFileIO, "%s", message).WithCause(err)
	}
	if status, ok := distribution.HTTPStatusCode(err); ok {
		subtype := errs.SubtypeNetworkProtocol
		retryable := false
		switch {
		case status == 408:
			subtype, retryable = errs.SubtypeNetworkTimeout, true
		case status >= 500:
			subtype, retryable = errs.SubtypeNetworkServer, true
		}
		networkErr := errs.NewNetworkError(subtype, "%s", message).WithCode(status).WithCause(err)
		if retryable {
			networkErr.WithRetryable()
		}
		return networkErr
	}
	subtype := errs.SubtypeNetworkProtocol
	retryable := false
	var netErr net.Error
	var dnsErr *net.DNSError
	var authorityErr x509.UnknownAuthorityError
	lower := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		subtype, retryable = errs.SubtypeNetworkTimeout, true
	case errors.As(err, &authorityErr), strings.Contains(lower, "x509:"), strings.Contains(lower, "tls:"):
		subtype = errs.SubtypeNetworkTLS
	case errors.As(err, &dnsErr):
		subtype, retryable = errs.SubtypeNetworkDNS, true
	case errors.As(err, &netErr):
		subtype, retryable = errs.SubtypeNetworkTransport, true
	}
	networkErr := errs.NewNetworkError(subtype, "%s", message).WithCause(err)
	if retryable && !errors.Is(err, context.Canceled) {
		networkErr.WithRetryable()
	}
	return networkErr
}
