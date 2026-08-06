// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// RenderMarkdown renders images and applies the requested GFM table limit.
func RenderMarkdown(resp *Response, maxRows int, hint string) string {
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return ""
	}
	content := RenderImages(resp.FullContent, resp.ImageMetaMap)
	content = TruncateGFMTables(content, maxRows, hint)
	return content
}

// FetchOptions configures rendering and pagination for one content read.
type FetchOptions struct {
	MaxRows   int
	Full      bool
	PageToken string
	PageSize  int
}

// FetchResult contains rendered Markdown, metadata, and pagination state.
type FetchResult struct {
	Content       string
	Title         string
	UpdateTime    int64
	HasMore       bool
	NextPageToken string
}

// FetchMarkdown reads and renders a URL-addressed non-document resource.
func FetchMarkdown(ctx context.Context, runtime *common.RuntimeContext, rawURL, fetchType string, opts FetchOptions) (*FetchResult, error) {
	req := NewRequest(rawURL)
	ApplyPagination(&req, opts.Full, opts.PageToken, opts.PageSize)
	resp, err := FetchDocInfo(ctx, runtime, req)
	if err != nil {
		return nil, err
	}
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"fetch response contained no readable content")
	}
	content := RenderMarkdown(resp, opts.MaxRows, TruncateHintFor(fetchType))
	return &FetchResult{
		Content:       content,
		Title:         resp.Title,
		UpdateTime:    resp.UpdateTime,
		HasMore:       resp.HasMore,
		NextPageToken: resp.NextPageToken,
	}, nil
}
