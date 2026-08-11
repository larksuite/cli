// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"context"
	"fmt"
)

const collectAllPagesLimit = 1000

// Page contains items and host-owned pagination state.
type Page[T any] struct {
	Items []T `json:"items" schema:"required;nonnullable" doc:"items returned by the API"`

	meta *paginationMeta
}

// Complete reports whether the API had no remaining page.
func (p Page[T]) Complete() bool { return p.meta != nil && p.meta.Complete }

// NextToken returns the next page token for an incomplete result.
func (p Page[T]) NextToken() string {
	if p.meta == nil {
		return ""
	}
	return p.meta.NextToken
}

// Pages returns the number of API pages collected.
func (p Page[T]) Pages() int {
	if p.meta == nil {
		return 0
	}
	return p.meta.Pages
}

func (p Page[T]) commandPagination() *paginationMeta { return p.meta }

type paginationMeta struct {
	Complete  bool
	Pages     int
	Items     int
	NextToken string
}

func clonePaginationMeta(meta *paginationMeta) *paginationMeta {
	if meta == nil {
		return nil
	}
	copy := *meta
	return &copy
}

type pageEnvelope[T any] struct {
	Items         []T    `json:"items"`
	HasMore       bool   `json:"has_more"`
	PageToken     string `json:"page_token"`
	NextPageToken string `json:"next_page_token"`
}

// CollectPages fetches one page by default or follows standard pagination flags.
func CollectPages[T any](ctx context.Context, command CommandContext, request Request) (Page[T], error) {
	options, err := command.pageOptions()
	if err != nil {
		return Page[T]{}, err
	}
	if !options.All {
		options.MaxPages = 1
	}
	return collectPages[T](ctx, command, request, options)
}

// CollectAllPages fetches until the endpoint is exhausted and ignores CLI paging flags.
func CollectAllPages[T any](ctx context.Context, command CommandContext, request Request) ([]T, error) {
	page, err := collectPages[T](ctx, command, request, PaginationOptions{All: true, MaxPages: collectAllPagesLimit})
	if err != nil {
		return nil, err
	}
	if !page.Complete() {
		return nil, PaginationLimitError(page.Pages(), page.NextToken())
	}
	return page.Items, nil
}

func collectPages[T any](ctx context.Context, command CommandContext, request Request, options PaginationOptions) (Page[T], error) {
	if options.MaxPages < 1 || options.MaxPages > collectAllPagesLimit {
		return Page[T]{}, ValidationErrorf("pagination page limit must be between 1 and %d", collectAllPagesLimit)
	}
	if options.Delay < 0 {
		return Page[T]{}, ValidationErrorf("pagination delay must not be negative")
	}

	result := Page[T]{meta: &paginationMeta{}}
	requestView := InspectRequest(request)
	token := queryPageToken(requestView.Query)
	seen := make(map[string]struct{}, options.MaxPages)
	if token != "" {
		seen[token] = struct{}{}
	}

	for pageNumber := 1; pageNumber <= options.MaxPages; pageNumber++ {
		pageRequest := request
		if token != "" {
			pageRequest = pageRequest.Set("page_token", token)
		}
		page, err := CallJSON[pageEnvelope[T]](ctx, command, pageRequest)
		if err != nil {
			result.meta.NextToken = token
			return result, err
		}
		result.Items = append(result.Items, page.Items...)
		result.meta.Pages++
		result.meta.Items = len(result.Items)

		nextToken := page.PageToken
		if nextToken == "" {
			nextToken = page.NextPageToken
		}
		if !page.HasMore {
			result.meta.Complete = true
			result.meta.NextToken = ""
			return result, nil
		}
		if nextToken == "" {
			return result, InvalidResponseErrorf("pagination page %d reports has_more=true without a page token", pageNumber)
		}
		if _, duplicate := seen[nextToken]; duplicate {
			return result, InvalidResponseErrorf("pagination page %d repeated page token %q", pageNumber, nextToken)
		}
		result.meta.NextToken = nextToken
		if pageNumber == options.MaxPages {
			return result, nil
		}
		seen[nextToken] = struct{}{}
		token = nextToken
		if err := waitForPage(ctx, options.Delay); err != nil {
			return result, err
		}
	}

	return result, InternalErrorf("pagination finished without a terminal state")
}

func queryPageToken(query map[string]any) string {
	value, ok := query["page_token"]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []string:
		if len(typed) > 0 {
			return typed[0]
		}
	case []any:
		if len(typed) > 0 {
			return fmt.Sprint(typed[0])
		}
	}
	return ""
}
