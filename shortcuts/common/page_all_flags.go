// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"time"

	"github.com/larksuite/cli/errs"
)

const (
	PageAllFlagName = "page-all"

	pageLimitFlagName = "page-limit"
	pageLimitDefault  = 10
	pageLimitMaximum  = 1000

	pageDelayFlagName = "page-delay"
	pageDelayDefault  = 200
	pageDelayMaximum  = 60_000
)

// PageAllPolicy makes potentially unbounded pagination an explicit caller
// decision. The zero value keeps --page-limit bounded; domains that can safely
// drain an endpoint must opt in to accepting 0 as "until exhaustion".
type PageAllPolicy struct {
	AllowUnlimited bool
}

// PageAllFlags returns the shared pagination control definitions. The caller's
// page token, when present, selects the starting cursor; --page-all controls
// whether pagination continues from that cursor until exhaustion or
// --page-limit. Each call returns a fresh slice so shortcuts cannot mutate each
// other.
func PageAllFlags(policy PageAllPolicy) []Flag {
	limitDesc := fmt.Sprintf("maximum pages fetched by --page-all (1-%d)", pageLimitMaximum)
	if policy.AllowUnlimited {
		limitDesc = fmt.Sprintf("maximum pages fetched by --page-all (0 = unlimited; otherwise 1-%d)", pageLimitMaximum)
	}
	return []Flag{
		{
			Name: PageAllFlagName,
			Type: "bool",
			Desc: "continue from --page-token (if set) until exhaustion or --page-limit",
		},
		{
			Name:    pageLimitFlagName,
			Type:    "int",
			Default: fmt.Sprintf("%d", pageLimitDefault),
			Desc:    limitDesc,
		},
		{
			Name:    pageDelayFlagName,
			Type:    "int",
			Default: fmt.Sprintf("%d", pageDelayDefault),
			Desc: fmt.Sprintf("delay in milliseconds between pages with --page-all (%d-%d; 0 disables throttling)",
				0, pageDelayMaximum),
		},
	}
}

// ValidatePageAllFlags validates the shared page budget and inter-page delay.
// PaginateInto repeats this check defensively for callers that invoke Execute
// directly in tests.
func ValidatePageAllFlags(runtime *RuntimeContext, policy PageAllPolicy) error {
	_, err := pageAllValues(runtime, policy)
	return err
}

type pageAllConfig struct {
	enabled  bool
	maxPages int
	delay    time.Duration
}

func pageAllValues(runtime *RuntimeContext, policy PageAllPolicy) (pageAllConfig, error) {
	if runtime == nil || runtime.Cmd == nil {
		return pageAllConfig{}, errs.NewInternalError(errs.SubtypeUnknown,
			"pagination requires a mounted shortcut command")
	}
	flags := runtime.Cmd.Flags()
	if flags.Lookup(PageAllFlagName) == nil || flags.Lookup(pageLimitFlagName) == nil || flags.Lookup(pageDelayFlagName) == nil {
		return pageAllConfig{}, errs.NewInternalError(errs.SubtypeUnknown,
			"pagination flags are not registered; append common.PageAllFlags() to the shortcut flags")
	}
	enabled, err := flags.GetBool(PageAllFlagName)
	if err != nil {
		return pageAllConfig{}, errs.NewInternalError(errs.SubtypeUnknown,
			"read pagination flag --%s: %v", PageAllFlagName, err).WithCause(err)
	}
	limit, err := flags.GetInt(pageLimitFlagName)
	if err != nil {
		return pageAllConfig{}, errs.NewInternalError(errs.SubtypeUnknown,
			"read pagination flag --%s: %v", pageLimitFlagName, err).WithCause(err)
	}
	minimum := 1
	if policy.AllowUnlimited {
		minimum = 0
	}
	if limit < minimum || limit > pageLimitMaximum {
		return pageAllConfig{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--%s must be an integer between %d and %d", pageLimitFlagName, minimum, pageLimitMaximum).
			WithParam("--" + pageLimitFlagName)
	}
	maxPages := limit
	if limit == 0 {
		// The caller explicitly opted into "read to exhaustion". The shared
		// walker still fails closed on a non-advancing server token.
		maxPages = int(^uint(0) >> 1)
	}
	delayMillis, err := flags.GetInt(pageDelayFlagName)
	if err != nil {
		return pageAllConfig{}, errs.NewInternalError(errs.SubtypeUnknown,
			"read pagination flag --%s: %v", pageDelayFlagName, err).WithCause(err)
	}
	if delayMillis < 0 || delayMillis > pageDelayMaximum {
		return pageAllConfig{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--%s must be an integer between 0 and %d", pageDelayFlagName, pageDelayMaximum).
			WithParam("--" + pageDelayFlagName)
	}
	return pageAllConfig{
		enabled:  enabled,
		maxPages: maxPages,
		delay:    time.Duration(delayMillis) * time.Millisecond,
	}, nil
}
