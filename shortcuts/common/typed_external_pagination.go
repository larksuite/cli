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

func commandPagePolicy(command CommandContext, all bool) (paginationPolicy, error) {
	if all {
		return paginationPolicy{maxPages: pageLimitMaximum}, nil
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
