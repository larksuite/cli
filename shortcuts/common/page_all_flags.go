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

// PageAllFlags returns the shared pagination control definitions.
// Each call returns a fresh slice so shortcuts cannot mutate each other.
func PageAllFlags() []Flag {
	return []Flag{
		{
			Name: PageAllFlagName,
			Type: "bool",
			Desc: "automatically paginate until exhaustion or --page-limit",
		},
		{
			Name:    pageLimitFlagName,
			Type:    "int",
			Default: fmt.Sprintf("%d", pageLimitDefault),
			Desc:    fmt.Sprintf("maximum pages fetched by --page-all (%d-%d)", 1, pageLimitMaximum),
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
func ValidatePageAllFlags(runtime *RuntimeContext) error {
	_, err := pageAllValues(runtime)
	return err
}

type pageAllConfig struct {
	enabled  bool
	maxPages int
	delay    time.Duration
}

func pageAllValues(runtime *RuntimeContext) (pageAllConfig, error) {
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
	if limit < 1 || limit > pageLimitMaximum {
		return pageAllConfig{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--%s must be an integer between 1 and %d", pageLimitFlagName, pageLimitMaximum).
			WithParam("--" + pageLimitFlagName)
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
		maxPages: limit,
		delay:    time.Duration(delayMillis) * time.Millisecond,
	}, nil
}
