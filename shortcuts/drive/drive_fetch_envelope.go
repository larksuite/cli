// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// fetchResource describes the resolved resource drive +fetch read from. Type is
// the canonical entity (doc, docx, sheet, bitable, slides, file, minutes, ...);
// Selector carries ?sheet=/?table= sub-resource selectors when present. Source
// records wiki provenance and is set only when the input was a wiki URL.
type fetchResource struct {
	Type             string            `json:"type"`
	Title            string            `json:"title,omitempty"`
	URL              string            `json:"url,omitempty"`
	Token            string            `json:"token,omitempty"`
	Selector         map[string]string `json:"selector,omitempty"`
	UpdateTime       int64             `json:"update_time,omitempty"` // omitempty: minutes carry none
	CreateTime       string            `json:"create_time,omitempty"` // omitempty: minutes only
	NoteID           string            `json:"note_id,omitempty"`
	NoteDocToken     string            `json:"note_doc_token,omitempty"`
	VerbatimDocToken string            `json:"verbatim_doc_token,omitempty"`
	Source           *fetchSource      `json:"source,omitempty"` // wiki provenance, nil unless input was a wiki URL
}

// fetchSource records that the input was a wiki node and how it unwrapped to the
// underlying resource. Emitted under resource.source so a caller can trace a
// read back to the wiki node it started from. When the get_node unwrap fails and
// the doc is read via direct fetch of the wiki URL, resource.type stays "wiki"
// (no unwrap happened) and source carries only InputURL (NodeToken/SpaceID absent).
type fetchSource struct {
	Type      string `json:"type"` // "wiki"
	InputURL  string `json:"input_url"`
	NodeToken string `json:"node_token,omitempty"`
	SpaceID   string `json:"space_id,omitempty"`
}

// fetchEnvelope is the unified drive +fetch output: inline Markdown or a local
// file descriptor, plus resource metadata for citations and follow-up reads.
// Internal routing details are intentionally omitted; resource.source records
// Wiki provenance only.
type fetchEnvelope struct {
	ContentDeliveryHint string                   `json:"content_delivery_hint,omitempty"`
	ContentInline       *bool                    `json:"content_inline,omitempty"`
	Content             *string                  `json:"content,omitempty"`
	ContentFile         *common.FetchContentFile `json:"content_file,omitempty"`
	ContentPreview      string                   `json:"content_preview,omitempty"`
	Resource            fetchResource            `json:"resource"`
	Warnings            []string                 `json:"warnings,omitempty"`
	HasMore             bool                     `json:"has_more,omitempty"`
	NextPageToken       string                   `json:"next_page_token,omitempty"`
}

// newFetchEnvelope starts a drive +fetch envelope; chain withPagination /
// withWarnings for the optional pagination cursor and warnings.
func newFetchEnvelope(content string, res fetchResource) *fetchEnvelope {
	return &fetchEnvelope{Content: &content, Resource: res}
}

// withContentDelivery returns a copy so the pre-scanned envelope remains
// immutable if a content-safety provider finishes after its timeout.
func (e fetchEnvelope) withContentDelivery(delivery common.FetchContentDelivery) *fetchEnvelope {
	if delivery.Inline() {
		if delivery.InlineHint != "" {
			inline := true
			e.ContentDeliveryHint = delivery.InlineHint
			e.ContentInline = &inline
		}
		return &e
	}
	inline := false
	e.Content = nil
	e.ContentInline = &inline
	e.ContentFile = delivery.File
	e.ContentPreview = delivery.Preview
	return &e
}

// withPagination records a pagination cursor so a --format json
// consumer sees has_more / next_page_token alongside the content. No-op when
// hasMore is false.
func (e *fetchEnvelope) withPagination(hasMore bool, nextToken string) *fetchEnvelope {
	e.HasMore = hasMore
	e.NextPageToken = strings.TrimSpace(nextToken)
	return e
}

// withWarnings appends non-empty warnings to the envelope.
func (e *fetchEnvelope) withWarnings(warnings ...string) *fetchEnvelope {
	for _, w := range warnings {
		if w = strings.TrimSpace(w); w != "" {
			e.Warnings = append(e.Warnings, w)
		}
	}
	return e
}
