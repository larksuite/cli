// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // intermediate draft attachment limit errors; mail command layer wraps into typed ValidationError.
package draft

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/mail/filecheck"
)

// Attachment limits mirrored from the parent mail package so that the draft
// sub-package can enforce the same constraints without a circular import.
const (
	maxAttachmentCount = 250
	maxAttachmentBytes = 25 * 1024 * 1024 // 25 MB
)

// checkBlockedExtension delegates to the shared filecheck package.
var checkBlockedExtension = filecheck.CheckBlockedExtension

// isAttachmentOrInline returns true if the part is an attachment or inline
// image (as opposed to a body text/plain or text/html part).
func isAttachmentOrInline(p *Part) bool {
	disp := strings.ToLower(p.ContentDisposition)
	return disp == "attachment" || disp == "inline" || p.ContentID != ""
}

// checkSnapshotAttachmentLimit verifies that adding a file of newFileSize bytes
// would not push the snapshot past the attachment count or total-size limits.
// replacedPart, if non-nil, is the part being replaced — its count and size
// are deducted so that a replace does not double-count.
//
// Callers must validate the file path via validate.SafeInputPath and stat the
// file themselves before calling this function.
func checkSnapshotAttachmentLimit(snapshot *DraftSnapshot, newFileSize int64, replacedPart *Part) error {
	var existingCount int
	var existingBytes int64
	for _, p := range flattenParts(snapshot.Body) {
		if p.IsMultipart() || !isAttachmentOrInline(p) {
			continue
		}
		existingCount++
		existingBytes += int64(len(p.Body))
	}

	totalCount := existingCount
	totalBytes := existingBytes + newFileSize
	if replacedPart != nil {
		totalBytes -= int64(len(replacedPart.Body))
	} else {
		totalCount++
	}

	if totalCount > maxAttachmentCount {
		return fmt.Errorf("attachment count %d exceeds the limit of %d", totalCount, maxAttachmentCount)
	}
	if totalBytes > maxAttachmentBytes {
		return fmt.Errorf("total attachment size %.1f MB exceeds the 25 MB limit",
			float64(totalBytes)/1024/1024)
	}
	return nil
}
