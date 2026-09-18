// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strings"
)

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

func (p Page[T]) commandPagination() *paginationMeta {
	meta := clonePaginationMeta(p.meta)
	if meta != nil {
		meta.Items = len(p.Items)
	}
	return meta
}

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

// pageItems extracts the single top-level array field of one page's data
// object, so upstream spellings (items, records, files, ...) all normalize
// into Page.Items. Zero or multiple array fields fail closed instead of
// silently dropping rows: pagination bookkeeping (has_more, page_token)
// would otherwise keep walking while every page decodes to nothing.
func pageItems[T any](data map[string]any) ([]T, error) {
	arrays := make([]string, 0, 1)
	for key, value := range data {
		if _, isArray := value.([]any); isArray {
			arrays = append(arrays, key)
		}
	}
	sort.Strings(arrays)
	if len(arrays) == 0 {
		return nil, InvalidResponseErrorf("pagination page has no top-level array field")
	}
	if len(arrays) > 1 {
		return nil, InvalidResponseErrorf("pagination page has multiple top-level array fields: %s", strings.Join(arrays, ", "))
	}
	encoded, err := json.Marshal(data[arrays[0]])
	if err != nil {
		return nil, err
	}
	var items []T
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&items); err != nil {
		return nil, err
	}
	return items, nil
}

// CollectPages fetches one page by default or follows standard pagination flags.
func CollectPages[T any](ctx context.Context, command CommandContext, request Request) (Page[T], error) {
	return collectPages[T](ctx, command, request, false)
}

// CollectAllPages fetches until the endpoint is exhausted and ignores CLI paging flags.
func CollectAllPages[T any](ctx context.Context, command CommandContext, request Request) ([]T, error) {
	page, err := collectPages[T](ctx, command, request, true)
	if err != nil {
		return nil, err
	}
	if !page.Complete() {
		return nil, PaginationLimitError(page.Pages(), page.NextToken())
	}
	return page.Items, nil
}

func collectPages[T any](ctx context.Context, command CommandContext, request Request, all bool) (Page[T], error) {
	// Items starts non-nil so a zero-item page encodes as [] rather than null:
	// the field is declared required;nonnullable, and a caller generating types
	// from that schema would reject the null.
	result := Page[T]{Items: make([]T, 0), meta: &paginationMeta{}}
	if err := validateRequest(request); err != nil {
		return result, err
	}
	if command.inputStage {
		return result, ValidationErrorf("network requests are unavailable in Normalize and Validate; move the call to Execute")
	}
	if command.dryRun {
		return result, ValidationErrorf("network requests are unavailable during dry-run")
	}
	if command.collectPages == nil {
		return result, InternalErrorf("command host does not provide pagination")
	}
	pages, pagination, err := command.collectPages(ctx, request, all)
	result.meta.Complete = pagination.Complete
	result.meta.Pages = pagination.Pages
	result.meta.NextToken = pagination.NextToken
	for pageNumber, data := range pages {
		items, decodeErr := pageItems[T](data)
		if decodeErr != nil {
			return result, InvalidResponseErrorf("decode pagination page %d: %v", pageNumber+1, decodeErr).WithCause(decodeErr)
		}
		result.Items = append(result.Items, items...)
	}
	result.meta.Items = len(result.Items)
	return result, err
}
