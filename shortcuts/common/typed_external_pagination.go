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

// CollectCommandPages uses the shared cursor walker for an externally declared command.
func CollectCommandPages(ctx context.Context, command CommandContext, request PageRequest, all bool) (CommandPageCollection, error) {
	policy, err := commandPagePolicy(command, all)
	if err != nil {
		return CommandPageCollection{}, err
	}
	collection := CommandPageCollection{}
	state, walkErr := internalpagination.Walk(ctx, internalpagination.Options{
		InitialToken: pageTokenParam(request.Params),
		MaxPages:     policy.maxPages,
		Delay:        policy.pageDelay,
		Fetch: func(ctx context.Context, _ int, pageToken string) (bool, string, error) {
			params := clonePageParams(request.Params)
			if pageToken != "" {
				params["page_token"] = pageToken
			}
			data, err := CallTypedAPI(ctx, command, request.Method, request.Path, params, request.Body)
			if err != nil {
				return false, "", err
			}
			collection.Data = append(collection.Data, data)
			hasMore, nextToken := PaginationMeta(data)
			return hasMore, nextToken, nil
		},
	})
	collection.Complete = state.Complete
	collection.Pages = state.Pages
	collection.NextToken = state.NextToken
	if walkErr != nil {
		return collection, paginationWalkError(walkErr)
	}
	return collection, nil
}

// collectAllHardPageBound caps CollectAllPages workflows. It is deliberately
// tighter than the user-facing --page-limit maximum (1000): a complete-set
// collection holds every page in memory before the workflow's writes run, so
// its upper bound is a host resource decision, not a display preference.
// Value from the extension design's Phase 0 (owner plan §8.3).
const collectAllHardPageBound = 100

func commandPagePolicy(command CommandContext, all bool) (paginationPolicy, error) {
	if all {
		return paginationPolicy{maxPages: collectAllHardPageBound}, nil
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
