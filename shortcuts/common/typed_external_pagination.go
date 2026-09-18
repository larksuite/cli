// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/commandbridge"
	"github.com/larksuite/cli/internal/output"
	internalpagination "github.com/larksuite/cli/internal/pagination"
)

// CollectHostedPages is PaginateInto for an externally declared command. Such
// a command compiles in the business module and so reaches the CLI through the
// CommandContext interface rather than a *RuntimeContext; the policy and the
// call arrive through that interface, and the context is a parameter because
// the interface carries none. Everything else is PaginateInto: the same cursor
// walk, the same per-page decode into T, the same accumulator, the same
// metadata.
//
// all selects the complete-set policy CollectAllPages needs -- collect to
// exhaustion under a hard page bound rather than obey --page-all and
// --page-limit. Built-in shortcuts have no equivalent, which is why it is a
// parameter here and not in PaginateInto.
func CollectHostedPages[T any](ctx context.Context, command typedRuntimeContext, request PageRequest, all bool, dst PageAccumulator[T], _ commandbridge.Access) (*output.PaginationMeta, error) {
	meta := &output.PaginationMeta{}
	policy, err := commandPagePolicy(command, all)
	if err != nil {
		return meta, err
	}
	state, walkErr := pageWalk{
		policy:  policy,
		request: request,
		fetch: func(ctx context.Context, page PageRequest) (map[string]interface{}, error) {
			return CallHostedAPI(ctx, command, page.Method, page.Path, page.Params, page.Body, commandbridge.Access{})
		},
		accumulate: func(data map[string]interface{}, pageNumber int) error {
			return addDecodedPage(data, pageNumber, dst)
		},
	}.run(ctx)
	meta.Complete = state.Complete
	meta.Pages = state.Pages
	meta.NextToken = state.NextToken
	return meta, walkErr
}

func commandPagePolicy(command typedRuntimeContext, all bool) (paginationPolicy, error) {
	if all {
		return paginationPolicy{maxPages: internalpagination.CollectAllHardPageBound}, nil
	}
	options, err := command.PaginationOptions()
	if err != nil {
		return paginationPolicy{}, err
	}
	if !options.All {
		options.MaxPages = 1
	}
	if options.MaxPages < 1 || options.MaxPages > pageLimitMaximum {
		return paginationPolicy{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"pagination page limit must be between 1 and %d", pageLimitMaximum)
	}
	if options.Delay < 0 || options.Delay > time.Duration(pageDelayMaximum)*time.Millisecond {
		return paginationPolicy{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"pagination delay must be between 0 and %d milliseconds", pageDelayMaximum)
	}
	return paginationPolicy{maxPages: options.MaxPages, pageDelay: options.Delay}, nil
}
