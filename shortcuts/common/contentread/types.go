// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package contentread implements the shared content-read client and Markdown
// rendering used by docs and drive +fetch.
package contentread

// Request is the subset of the content-read request used by the CLI.
// WithBlockID selects anchored XML; pagination fields are omitted when unused.
type Request struct {
	URL              string `json:"url"`
	WithBlockID      bool   `json:"with_block_id,omitempty"`
	EnablePagination bool   `json:"enable_pagination,omitempty"`
	PageToken        string `json:"page_token,omitempty"`
	PageSize         int32  `json:"page_size,omitempty"`
}

// NewRequest builds a request from the verbatim resource URL so selectors such
// as ?sheet= and ?table= reach the service.
func NewRequest(rawURL string) Request {
	return Request{URL: rawURL}
}

// Response is the subset of the content-read response consumed by the CLI.
// FullContent contains anchored XML when WithBlockID was requested and Markdown
// otherwise; HasMore and NextPageToken describe the current page.
type Response struct {
	Title         string                `json:"title"`
	FullContent   string                `json:"full_content"`
	URL           string                `json:"url"`
	UpdateTime    int64                 `json:"update_time"`
	ImageMetaMap  map[string]*ImageMeta `json:"qa_image_meta_map"`
	NextPageToken string                `json:"next_page_token"`
	HasMore       bool                  `json:"has_more"`
}

// ImageMeta contains the image fields currently returned by content-read.
type ImageMeta struct {
	ImageKey string `json:"image_key"`
	Caption  string `json:"caption"`
}
