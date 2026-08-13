// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"time"

	"github.com/larksuite/cli/errs"
	internalpagination "github.com/larksuite/cli/internal/pagination"
)

// CommandPageCollection is the host projection used by the public command adapter.
type CommandPageCollection struct {
	Data      []map[string]any
	Complete  bool
	Pages     int
	NextToken string
}

// CollectCommandPages walks pages for an externally declared command. It runs
// the same pageWalk built-in shortcuts use; only the policy source, the call
// path and the accumulator differ. Pages stay undecoded here because the public
// command contract decodes them into its own Page[T].
func CollectCommandPages(ctx context.Context, command CommandContext, request PageRequest, all bool) (CommandPageCollection, error) {
	policy, err := commandPagePolicy(command, all)
	if err != nil {
		return CommandPageCollection{}, err
	}
	collection := CommandPageCollection{}
	state, walkErr := pageWalk{
		policy:  policy,
		request: request,
		fetch: func(ctx context.Context, page PageRequest) (map[string]interface{}, error) {
			return CallTypedAPI(ctx, command, page.Method, page.Path, page.Params, page.Body)
		},
		accumulate: func(data map[string]interface{}, _ int) error {
			collection.Data = append(collection.Data, data)
			return nil
		},
	}.run(ctx)
	collection.Complete = state.Complete
	collection.Pages = state.Pages
	collection.NextToken = state.NextToken
	return collection, walkErr
}

func commandPagePolicy(command CommandContext, all bool) (paginationPolicy, error) {
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
