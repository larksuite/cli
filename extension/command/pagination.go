// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"bytes"
	"context"
	"encoding/json"
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

type pageEnvelope[T any] struct {
	Items         []T    `json:"items"`
	HasMore       bool   `json:"has_more"`
	PageToken     string `json:"page_token"`
	NextPageToken string `json:"next_page_token"`
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
	result := Page[T]{meta: &paginationMeta{}}
	if err := validateRequest(request); err != nil {
		return result, err
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
		page, decodeErr := decodePageEnvelope[T](data)
		if decodeErr != nil {
			return result, InvalidResponseErrorf("decode pagination page %d: %v", pageNumber+1, decodeErr).WithCause(decodeErr)
		}
		result.Items = append(result.Items, page.Items...)
	}
	result.meta.Items = len(result.Items)
	return result, err
}

func decodePageEnvelope[T any](data map[string]any) (pageEnvelope[T], error) {
	var page pageEnvelope[T]
	encoded, err := json.Marshal(data)
	if err != nil {
		return page, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&page); err != nil {
		return page, err
	}
	return page, nil
}
