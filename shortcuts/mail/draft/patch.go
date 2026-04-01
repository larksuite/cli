// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/mail/filecheck"
)

var protectedHeaders = map[string]bool{
	"message-id":                true,
	"mime-version":              true,
	"content-type":              true,
	"content-transfer-encoding": true,
	"in-reply-to":               true,
	"references":                true,
	"reply-to":                  true,
}

func Apply(snapshot *DraftSnapshot, patch Patch) error {
	if err := patch.Validate(); err != nil {
		return err
	}
	for _, op := range patch.Ops {
		if err := applyOp(snapshot, op, patch.Options); err != nil {
			return err
		}
	}
	if err := refreshSnapshot(snapshot); err != nil {
		return err
	}
	if err := validateInlineCIDAfterApply(snapshot); err != nil {
		return err
	}
	return validateOrphanedInlineCIDAfterApply(snapshot)
}

func applyOp(snapshot *DraftSnapshot, op PatchOp, options PatchOptions) error {
	switch op.Op {
	case "set_subject":
		if strings.ContainsAny(op.Value, "\r\n") {
			return fmt.Errorf("set_subject: value must not contain CR or LF")
		}
		upsertHeader(&snapshot.Headers, "Subject", op.Value)
	case "set_recipients":
		return setRecipients(snapshot, op.Field, op.Addresses)
	case "add_recipient":
		return addRecipient(snapshot, op.Field, Address{Name: op.Name, Address: op.Address})
	case "remove_recipient":
		return removeRecipient(snapshot, op.Field, op.Address)
	case "set_reply_to":
		upsertHeader(&snapshot.Headers, "Reply-To", formatAddressList(op.Addresses))
	case "clear_reply_to":
		removeHeader(&snapshot.Headers, "Reply-To")
	case "set_body":
		return setBody(snapshot, op.Value, options)
	case "set_reply_body":
		return setReplyBody(snapshot, op.Value, options)
	case "replace_body":
		return replaceBody(snapshot, op.BodyKind, op.Value, options)
	case "append_body":
		return appendBody(snapshot, op.BodyKind, op.Value, options)
	case "set_header":
		if err := ensureHeaderEditable(op.Name, options); err != nil {
			return err
		}
		if strings.ContainsAny(op.Name, ":\r\n") {
			return fmt.Errorf("set_header: header name must not contain ':', CR, or LF")
		}
		if strings.ContainsAny(op.Value, "\r\n") {
			return fmt.Errorf("set_header: header value must not contain CR or LF")
		}
		upsertHeader(&snapshot.Headers, op.Name, op.Value)
	case "remove_header":
		if err := ensureHeaderEditable(op.Name, options); err != nil {
			return err
		}
		removeHeader(&snapshot.Headers, op.Name)
	case "add_attachment":
		return addAttachment(snapshot, op.Path)
	case "remove_attachment":
		partID, err := resolveTarget(snapshot, op.Target)
		if err != nil {
			return fmt.Errorf("remove_attachment: %w", err)
		}
		return removeAttachment(snapshot, partID)
	case "add_inline":
		return addInline(snapshot, op.Path, op.CID, op.FileName, op.ContentType)
	case "replace_inline":
		partID, err := resolveTarget(snapshot, op.Target)
		if err != nil {
			return fmt.Errorf("replace_inline: %w", err)
		}
		return replaceInline(snapshot, partID, op.Path, op.CID, op.FileName, op.ContentType)
	case "remove_inline":
		partID, err := resolveTarget(snapshot, op.Target)
		if err != nil {
			return fmt.Errorf("remove_inline: %w", err)
		}
		return removeInline(snapshot, partID)
	default:
		return fmt.Errorf("unsupported patch op %q", op.Op)
	}
	return nil
}

func ensureHeaderEditable(name string, options PatchOptions) error {
	if protectedHeaders[strings.ToLower(strings.TrimSpace(name))] && !options.AllowProtectedHeaderEdits {
		return fmt.Errorf("header %q is protected; rerun with allow_protected_header_edits", name)
	}
	return nil
}

func setRecipients(snapshot *DraftSnapshot, field string, addrs []Address) error {
	field = strings.ToLower(strings.TrimSpace(field))
	if !isRecipientField(field) {
		return fmt.Errorf("recipient field must be one of to/cc/bcc")
	}
	normalized := make([]Address, 0, len(addrs))
	seen := map[string]bool{}
	for _, addr := range addrs {
		if strings.TrimSpace(addr.Address) == "" {
			return fmt.Errorf("recipient address is empty")
		}
		key := strings.ToLower(strings.TrimSpace(addr.Address))
		if seen[key] {
			continue
		}
		seen[key] = true
		normalized = append(normalized, Address{
			Name:    addr.Name,
			Address: strings.TrimSpace(addr.Address),
		})
	}
	_, headerName := recipientField(snapshot, field)
	setRecipientField(snapshot, headerName, normalized)
	return nil
}

func addRecipient(snapshot *DraftSnapshot, field string, addr Address) error {
	if strings.TrimSpace(addr.Address) == "" {
		return fmt.Errorf("recipient address is empty")
	}
	field = strings.ToLower(strings.TrimSpace(field))
	addrs, headerName := recipientField(snapshot, field)
	key := strings.ToLower(strings.TrimSpace(addr.Address))
	seen := false
	for _, existing := range addrs {
		if strings.EqualFold(existing.Address, key) || strings.EqualFold(existing.Address, addr.Address) {
			seen = true
			break
		}
	}
	if !seen {
		addrs = append(addrs, addr)
	}
	setRecipientField(snapshot, headerName, addrs)
	return nil
}

func removeRecipient(snapshot *DraftSnapshot, field, address string) error {
	field = strings.ToLower(strings.TrimSpace(field))
	addrs, headerName := recipientField(snapshot, field)
	if len(addrs) == 0 {
		return fmt.Errorf("%s header is empty", headerName)
	}
	needle := strings.ToLower(strings.TrimSpace(address))
	next := make([]Address, 0, len(addrs))
	removed := false
	for _, addr := range addrs {
		if strings.EqualFold(strings.TrimSpace(addr.Address), needle) {
			removed = true
			continue
		}
		next = append(next, addr)
	}
	if !removed {
		return fmt.Errorf("recipient %q not found in %s", address, headerName)
	}
	setRecipientField(snapshot, headerName, next)
	return nil
}

func recipientField(snapshot *DraftSnapshot, field string) ([]Address, string) {
	switch field {
	case "to":
		return append([]Address{}, snapshot.To...), "To"
	case "cc":
		return append([]Address{}, snapshot.Cc...), "Cc"
	case "bcc":
		return append([]Address{}, snapshot.Bcc...), "Bcc"
	default:
		return nil, ""
	}
}

func setRecipientField(snapshot *DraftSnapshot, headerName string, addrs []Address) {
	if len(addrs) == 0 {
		removeHeader(&snapshot.Headers, headerName)
		return
	}
	upsertHeader(&snapshot.Headers, headerName, formatAddressList(addrs))
}

func replaceBody(snapshot *DraftSnapshot, bodyKind, value string, options PatchOptions) error {
	if hasCoupledBodySummary(snapshot) {
		return fmt.Errorf("draft has coupled text/plain summary and text/html body; edit them together with set_body")
	}
	part, err := bodyPartForKind(snapshot, bodyKind, options.RewriteEntireDraft)
	if err != nil {
		return err
	}
	part.Body = []byte(value)
	part.Dirty = true
	return nil
}

func appendBody(snapshot *DraftSnapshot, bodyKind, value string, options PatchOptions) error {
	if hasCoupledBodySummary(snapshot) {
		return fmt.Errorf("draft has coupled text/plain summary and text/html body; edit them together with set_body")
	}
	part, err := bodyPartForKind(snapshot, bodyKind, options.RewriteEntireDraft)
	if err != nil {
		return err
	}
	part.Body = append(part.Body, []byte(value)...)
	part.Dirty = true
	return nil
}

func setBody(snapshot *DraftSnapshot, value string, options PatchOptions) error {
	switch {
	case snapshot.PrimaryTextPartID != "" && snapshot.PrimaryHTMLPartID == "":
		return replaceBody(snapshot, "text/plain", value, options)
	case snapshot.PrimaryTextPartID == "" && snapshot.PrimaryHTMLPartID != "":
		return replaceBody(snapshot, "text/html", value, options)
	case snapshot.PrimaryTextPartID != "" && snapshot.PrimaryHTMLPartID != "":
		if err := coupledBodySetBodyInputError(snapshot, value); err != nil {
			return err
		}
		if tryApplyCoupledBodySetBody(snapshot, value) {
			return nil
		}
		return fmt.Errorf("draft has both text/plain and text/html body parts, but they are not a supported summary+html pair")
	default:
		return fmt.Errorf("draft has no unique primary body part; use replace_body with body_kind")
	}
}

// setReplyBody replaces only the user-authored portion of the HTML body,
// preserving the trailing reply/forward quote block (the
// history-quote-wrapper div generated by +reply / +forward). If no quote
// block is found, it falls back to setBody.
func setReplyBody(snapshot *DraftSnapshot, value string, options PatchOptions) error {
	htmlPartID := snapshot.PrimaryHTMLPartID
	if htmlPartID == "" {
		// No HTML part — fall back to setBody which handles text-only drafts.
		return setBody(snapshot, value, options)
	}
	htmlPart := findPart(snapshot.Body, htmlPartID)
	if htmlPart == nil {
		return setBody(snapshot, value, options)
	}
	_, quotePart := splitAtQuote(string(htmlPart.Body))
	if quotePart == "" {
		// No quote block found — fall back to regular set_body.
		return setBody(snapshot, value, options)
	}
	// Combine the new user content with the preserved quote block.
	return setBody(snapshot, value+quotePart, options)
}

func tryApplyCoupledBodySetBody(snapshot *DraftSnapshot, value string) bool {
	textPart := findPart(snapshot.Body, snapshot.PrimaryTextPartID)
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	if textPart == nil || htmlPart == nil {
		return false
	}
	if !strings.EqualFold(textPart.MediaType, "text/plain") || !strings.EqualFold(htmlPart.MediaType, "text/html") {
		return false
	}

	htmlPart.Body = []byte(value)
	htmlPart.Dirty = true
	textPart.Body = []byte(plainTextFromHTML(value))
	textPart.Dirty = true
	return true
}

func hasCoupledBodySummary(snapshot *DraftSnapshot) bool {
	if snapshot == nil {
		return false
	}
	textPart := findPart(snapshot.Body, snapshot.PrimaryTextPartID)
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	if textPart == nil || htmlPart == nil {
		return false
	}
	return strings.EqualFold(textPart.MediaType, "text/plain") && strings.EqualFold(htmlPart.MediaType, "text/html")
}

func coupledBodySetBodyInputError(snapshot *DraftSnapshot, value string) error {
	if !hasCoupledBodySummary(snapshot) {
		return nil
	}
	if bodyLooksLikeHTML(value) {
		return nil
	}
	return fmt.Errorf("draft main body is text/html and text/plain is only its summary; set_body requires HTML input for this draft")
}

func bodyPartForKind(snapshot *DraftSnapshot, bodyKind string, allowRewrite bool) (*Part, error) {
	var partID string
	switch strings.ToLower(bodyKind) {
	case "text/plain":
		partID = snapshot.PrimaryTextPartID
	case "text/html":
		partID = snapshot.PrimaryHTMLPartID
	default:
		return nil, fmt.Errorf("unsupported body kind %q", bodyKind)
	}
	if partID == "" {
		if !allowRewrite {
			return nil, fmt.Errorf("draft has no primary %s body part", bodyKind)
		}
		return ensureBodyPart(snapshot, bodyKind)
	}
	part := findPart(snapshot.Body, partID)
	if part == nil {
		return nil, fmt.Errorf("body part %s not found", partID)
	}
	return part, nil
}

func ensureBodyPart(snapshot *DraftSnapshot, bodyKind string) (*Part, error) {
	partRef := primaryBodyRootRef(&snapshot.Body)
	if partRef == nil {
		return nil, fmt.Errorf("draft has no primary body container")
	}
	return ensureBodyPartRef(partRef, bodyKind)
}

func primaryBodyRootRef(root **Part) **Part {
	if root == nil || *root == nil {
		return root
	}
	part := *root
	if strings.EqualFold(part.MediaType, "multipart/mixed") {
		for idx := range part.Children {
			child := part.Children[idx]
			if child == nil || strings.EqualFold(child.ContentDisposition, "attachment") {
				continue
			}
			return &part.Children[idx]
		}
		if len(part.Children) == 0 {
			part.Children = append(part.Children, nil)
			return &part.Children[0]
		}
	}
	return root
}

func ensureBodyPartRef(partRef **Part, bodyKind string) (*Part, error) {
	if partRef == nil {
		return nil, fmt.Errorf("body container is nil")
	}
	if *partRef == nil {
		leaf := newBodyLeaf(bodyKind)
		leaf.Dirty = true
		*partRef = leaf
		return leaf, nil
	}
	part := *partRef
	if !part.IsMultipart() {
		if strings.EqualFold(part.MediaType, bodyKind) {
			return part, nil
		}
		if !isBodyKind(part.MediaType) {
			return nil, fmt.Errorf("cannot rewrite non-body media type %q", part.MediaType)
		}
		newLeaf := newBodyLeaf(bodyKind)
		alt := newMultipartContainer("multipart/alternative")
		if strings.EqualFold(part.MediaType, "text/plain") {
			alt.Children = []*Part{part, newLeaf}
		} else {
			alt.Children = []*Part{newLeaf, part}
		}
		alt.Dirty = true
		newLeaf.Dirty = true
		*partRef = alt
		return newLeaf, nil
	}

	switch strings.ToLower(part.MediaType) {
	case "multipart/alternative":
		for _, child := range part.Children {
			if child != nil && strings.EqualFold(child.MediaType, bodyKind) {
				return child, nil
			}
		}
		newLeaf := newBodyLeaf(bodyKind)
		if strings.EqualFold(bodyKind, "text/plain") {
			part.Children = append([]*Part{newLeaf}, part.Children...)
		} else {
			part.Children = append(part.Children, newLeaf)
		}
		part.Dirty = true
		newLeaf.Dirty = true
		return newLeaf, nil
	case "multipart/related":
		for idx := range part.Children {
			child := part.Children[idx]
			if child == nil {
				continue
			}
			if child.IsMultipart() && strings.EqualFold(child.MediaType, "multipart/alternative") {
				return ensureBodyPartRef(&part.Children[idx], bodyKind)
			}
		}
		if len(part.Children) == 0 {
			leaf := newBodyLeaf(bodyKind)
			part.Children = append(part.Children, leaf)
			part.Dirty = true
			leaf.Dirty = true
			return leaf, nil
		}
		return ensureBodyPartRef(&part.Children[0], bodyKind)
	default:
		return nil, fmt.Errorf("rewrite_entire_draft cannot synthesize body inside %q", part.MediaType)
	}
}

func newBodyLeaf(bodyKind string) *Part {
	return &Part{
		MediaType:        strings.ToLower(bodyKind),
		MediaParams:      map[string]string{"charset": "UTF-8"},
		TransferEncoding: "7bit",
		Headers: []Header{
			{Name: "Content-Type", Value: mime.FormatMediaType(strings.ToLower(bodyKind), map[string]string{"charset": "UTF-8"})},
			{Name: "Content-Transfer-Encoding", Value: "7bit"},
		},
		Body: []byte{},
	}
}

func newMultipartContainer(mediaType string) *Part {
	boundary := newBoundary()
	return &Part{
		MediaType:   strings.ToLower(mediaType),
		MediaParams: map[string]string{"boundary": boundary},
		Headers: []Header{
			{Name: "Content-Type", Value: mime.FormatMediaType(strings.ToLower(mediaType), map[string]string{"boundary": boundary})},
		},
	}
}

func addAttachment(snapshot *DraftSnapshot, path string) error {
	safePath, err := validate.SafeInputPath(path)
	if err != nil {
		return fmt.Errorf("attachment %q: %w", path, err)
	}
	if err := checkBlockedExtension(filepath.Base(path)); err != nil {
		return err
	}
	info, err := os.Stat(safePath)
	if err != nil {
		return err
	}
	if err := checkSnapshotAttachmentLimit(snapshot, info.Size(), nil); err != nil {
		return err
	}
	content, err := os.ReadFile(safePath)
	if err != nil {
		return err
	}
	filename := filepath.Base(path)
	contentType := "application/octet-stream"
	mediaParams := map[string]string{}
	mediaParams["name"] = filename
	attachment := &Part{
		MediaType:             contentType,
		MediaParams:           mediaParams,
		ContentDisposition:    "attachment",
		ContentDispositionArg: map[string]string{"filename": filename},
		TransferEncoding:      "base64",
		Body:                  content,
		Headers: []Header{
			{Name: "Content-Type", Value: mime.FormatMediaType(contentType, cloneStringMap(mediaParams))},
			{Name: "Content-Disposition", Value: mime.FormatMediaType("attachment", map[string]string{"filename": filename})},
			{Name: "Content-Transfer-Encoding", Value: "base64"},
		},
	}

	if snapshot.Body == nil {
		snapshot.Body = attachment
		snapshot.Body.Dirty = true
		return nil
	}
	if strings.EqualFold(snapshot.Body.MediaType, "multipart/mixed") {
		snapshot.Body.Children = append(snapshot.Body.Children, attachment)
		snapshot.Body.Dirty = true
		return nil
	}
	boundary := newBoundary()
	original := snapshot.Body
	snapshot.Body = &Part{
		MediaType:   "multipart/mixed",
		MediaParams: map[string]string{"boundary": boundary},
		Dirty:       true,
		Headers: []Header{
			{Name: "Content-Type", Value: mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": boundary})},
		},
		Children: []*Part{original, attachment},
	}
	return nil
}

func addInline(snapshot *DraftSnapshot, path, cid, fileName, contentType string) error {
	safePath, err := validate.SafeInputPath(path)
	if err != nil {
		return fmt.Errorf("inline image %q: %w", path, err)
	}
	info, err := os.Stat(safePath)
	if err != nil {
		return err
	}
	if err := checkSnapshotAttachmentLimit(snapshot, info.Size(), nil); err != nil {
		return err
	}
	content, err := os.ReadFile(safePath)
	if err != nil {
		return err
	}
	name := fileName
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(path)
	}
	detectedCT, err := filecheck.CheckInlineImageFormat(name, content)
	if err != nil {
		return err
	}
	inline, err := newInlinePart(path, content, cid, fileName, detectedCT)
	if err != nil {
		return err
	}
	containerRef := primaryBodyRootRef(&snapshot.Body)
	if containerRef == nil || *containerRef == nil {
		return fmt.Errorf("draft has no primary body container")
	}
	container, err := ensureInlineContainerRef(containerRef)
	if err != nil {
		return err
	}
	container.Children = append(container.Children, inline)
	container.Dirty = true
	return nil
}

func replaceInline(snapshot *DraftSnapshot, partID, path, cid, fileName, contentType string) error {
	part := findPart(snapshot.Body, partID)
	if part == nil {
		return fmt.Errorf("inline part %q not found", partID)
	}
	if !isInlinePart(part) {
		return fmt.Errorf("part %q is not an inline MIME part", partID)
	}
	safePath, err := validate.SafeInputPath(path)
	if err != nil {
		return fmt.Errorf("inline image %q: %w", path, err)
	}
	info, err := os.Stat(safePath)
	if err != nil {
		return err
	}
	if err := checkSnapshotAttachmentLimit(snapshot, info.Size(), part); err != nil {
		return err
	}
	content, err := os.ReadFile(safePath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = part.FileName()
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = part.MediaType
	}
	if strings.TrimSpace(cid) == "" {
		cid = part.ContentID
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = filepath.Base(path)
	}
	detectedCT, err := filecheck.CheckInlineImageFormat(fileName, content)
	if err != nil {
		return err
	}
	contentType = detectedCT
	contentType, mediaParams := normalizedDetectedMediaType(contentType)
	finalCID := strings.Trim(strings.TrimSpace(cid), "<>")
	if err := validate.RejectCRLF(finalCID, "inline cid"); err != nil {
		return err
	}
	if err := validate.RejectCRLF(fileName, "inline filename"); err != nil {
		return err
	}
	mediaParams["name"] = fileName
	part.MediaType = contentType
	part.MediaParams = mediaParams
	part.ContentDisposition = "inline"
	part.ContentDispositionArg = map[string]string{"filename": fileName}
	part.ContentID = finalCID
	part.TransferEncoding = "base64"
	part.Body = content
	part.Dirty = true
	syncStructuredPartHeaders(part)
	return nil
}

func removeInline(snapshot *DraftSnapshot, partID string) error {
	part := findPart(snapshot.Body, partID)
	if part == nil {
		return fmt.Errorf("inline part %q not found", partID)
	}
	if !isInlinePart(part) {
		return fmt.Errorf("part %q is not an inline MIME part", partID)
	}
	if snapshot.Body == nil || snapshot.Body.PartID == partID {
		return fmt.Errorf("cannot remove root MIME part")
	}
	if !removePart(snapshot.Body, partID) {
		return fmt.Errorf("inline part %q not found", partID)
	}
	return nil
}

func removeAttachment(snapshot *DraftSnapshot, partID string) error {
	if snapshot.Body == nil {
		return fmt.Errorf("draft has no MIME body")
	}
	part := findPart(snapshot.Body, partID)
	if part == nil {
		return fmt.Errorf("attachment part %q not found", partID)
	}
	if strings.EqualFold(part.ContentDisposition, "inline") || part.ContentID != "" {
		return fmt.Errorf("part %q is an inline MIME part; use remove_inline", partID)
	}
	if snapshot.Body.PartID == partID {
		return fmt.Errorf("cannot remove root MIME part")
	}
	removed := removePart(snapshot.Body, partID)
	if !removed {
		return fmt.Errorf("attachment part %q not found", partID)
	}
	return nil
}

func removePart(parent *Part, targetPartID string) bool {
	for idx, child := range parent.Children {
		if child == nil {
			continue
		}
		if child.PartID == targetPartID {
			parent.Children = append(parent.Children[:idx], parent.Children[idx+1:]...)
			parent.Dirty = true
			return true
		}
		if removePart(child, targetPartID) {
			parent.Dirty = true
			return true
		}
	}
	return false
}

// resolveTarget resolves an AttachmentTarget to a concrete part_id.
// Priority: part_id > cid.
func resolveTarget(snapshot *DraftSnapshot, target AttachmentTarget) (string, error) {
	if id := strings.TrimSpace(target.PartID); id != "" {
		return id, nil
	}
	if cid := strings.TrimSpace(target.CID); cid != "" {
		cid = strings.Trim(cid, "<>")
		part := findPartByCID(snapshot.Body, cid)
		if part == nil {
			return "", fmt.Errorf("no part with cid %q found", cid)
		}
		return part.PartID, nil
	}
	return "", fmt.Errorf("target must specify at least one of part_id or cid")
}

func findPartByCID(root *Part, cid string) *Part {
	if root == nil {
		return nil
	}
	if strings.EqualFold(strings.Trim(root.ContentID, "<>"), cid) {
		return root
	}
	for _, child := range root.Children {
		if found := findPartByCID(child, cid); found != nil {
			return found
		}
	}
	return nil
}

func findPart(root *Part, partID string) *Part {
	if root == nil {
		return nil
	}
	if root.PartID == partID {
		return root
	}
	for _, child := range root.Children {
		if child == nil {
			continue
		}
		if found := findPart(child, partID); found != nil {
			return found
		}
	}
	return nil
}

func ensureInlineContainerRef(partRef **Part) (*Part, error) {
	if partRef == nil || *partRef == nil {
		return nil, fmt.Errorf("body container is nil")
	}
	part := *partRef
	if strings.EqualFold(part.MediaType, "multipart/related") {
		return part, nil
	}
	related := newMultipartContainer("multipart/related")
	related.Children = []*Part{part}
	related.Dirty = true
	*partRef = related
	return related, nil
}

func newInlinePart(path string, content []byte, cid, fileName, contentType string) (*Part, error) {
	if strings.TrimSpace(fileName) == "" {
		fileName = filepath.Base(path)
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = mime.TypeByExtension(filepath.Ext(fileName))
	}
	contentType, mediaParams := normalizedDetectedMediaType(contentType)
	mediaParams["name"] = fileName
	cid = strings.Trim(strings.TrimSpace(cid), "<>")
	if cid == "" {
		return nil, fmt.Errorf("inline cid is empty")
	}
	if err := validate.RejectCRLF(cid, "inline cid"); err != nil {
		return nil, err
	}
	if err := validate.RejectCRLF(fileName, "inline filename"); err != nil {
		return nil, err
	}
	part := &Part{
		MediaType:             contentType,
		MediaParams:           mediaParams,
		ContentDisposition:    "inline",
		ContentDispositionArg: map[string]string{"filename": fileName},
		ContentID:             cid,
		TransferEncoding:      "base64",
		Body:                  content,
		Dirty:                 true,
	}
	syncStructuredPartHeaders(part)
	return part, nil
}

func normalizedDetectedMediaType(detected string) (string, map[string]string) {
	detected = strings.TrimSpace(detected)
	if detected == "" {
		return "application/octet-stream", map[string]string{}
	}
	mediaType, params, err := mime.ParseMediaType(detected)
	if err != nil || strings.TrimSpace(mediaType) == "" {
		return detected, map[string]string{}
	}
	normalized := lowerCaseKeys(params)
	if normalized == nil {
		normalized = map[string]string{}
	}
	return mediaType, normalized
}

func syncStructuredPartHeaders(part *Part) {
	if part == nil {
		return
	}
	headers := make([]Header, 0, len(part.Headers)+4)
	for _, header := range part.Headers {
		switch strings.ToLower(header.Name) {
		case "content-type", "content-transfer-encoding", "content-disposition", "content-id":
			continue
		default:
			headers = append(headers, header)
		}
	}
	headers = append(headers, Header{Name: "Content-Type", Value: mime.FormatMediaType(part.MediaType, cloneStringMap(part.MediaParams))})
	if part.ContentDisposition != "" {
		headers = append(headers, Header{Name: "Content-Disposition", Value: mime.FormatMediaType(part.ContentDisposition, cloneStringMap(part.ContentDispositionArg))})
	}
	if part.ContentID != "" {
		headers = append(headers, Header{Name: "Content-ID", Value: "<" + part.ContentID + ">"})
	}
	if part.TransferEncoding != "" {
		headers = append(headers, Header{Name: "Content-Transfer-Encoding", Value: part.TransferEncoding})
	}
	part.Headers = headers
}

func isInlinePart(part *Part) bool {
	if part == nil {
		return false
	}
	return strings.EqualFold(part.ContentDisposition, "inline") || strings.TrimSpace(part.ContentID) != ""
}

func upsertHeader(headers *[]Header, name, value string) {
	for i, header := range *headers {
		if strings.EqualFold(header.Name, name) {
			(*headers)[i].Value = value
			j := i + 1
			for j < len(*headers) {
				if strings.EqualFold((*headers)[j].Name, name) {
					*headers = append((*headers)[:j], (*headers)[j+1:]...)
					continue
				}
				j++
			}
			return
		}
	}
	*headers = append(*headers, Header{Name: name, Value: value})
}

func removeHeader(headers *[]Header, name string) {
	next := (*headers)[:0]
	for _, header := range *headers {
		if strings.EqualFold(header.Name, name) {
			continue
		}
		next = append(next, header)
	}
	*headers = next
}

// validateInlineCIDAfterApply checks that all CID references in the HTML body
// resolve to actual inline MIME parts. This is called after Apply (editing) to
// prevent broken CID references, but NOT during Parse (where broken CIDs
// should not block opening the draft).
func validateInlineCIDAfterApply(snapshot *DraftSnapshot) error {
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	if htmlPart == nil {
		return nil
	}
	refs := extractCIDRefs(string(htmlPart.Body))
	if len(refs) == 0 {
		return nil
	}
	cids := make(map[string]bool)
	for _, part := range flattenParts(snapshot.Body) {
		if part == nil || part.ContentID == "" {
			continue
		}
		cids[strings.ToLower(part.ContentID)] = true
	}
	for _, ref := range refs {
		if !cids[strings.ToLower(ref)] {
			return fmt.Errorf("html body references missing inline cid %q", ref)
		}
	}
	return nil
}

// validateOrphanedInlineCIDAfterApply checks the reverse direction: every
// inline MIME part with a ContentID must be referenced by the HTML body.
// An orphaned inline part (CID exists but HTML has no <img src="cid:...">) will
// be displayed as an unexpected attachment by most mail clients.
func validateOrphanedInlineCIDAfterApply(snapshot *DraftSnapshot) error {
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	if htmlPart == nil {
		return nil
	}
	refs := extractCIDRefs(string(htmlPart.Body))
	refSet := make(map[string]bool, len(refs))
	for _, ref := range refs {
		refSet[strings.ToLower(ref)] = true
	}
	var orphaned []string
	for _, part := range flattenParts(snapshot.Body) {
		if part == nil || part.ContentID == "" {
			continue
		}
		if !refSet[strings.ToLower(part.ContentID)] {
			orphaned = append(orphaned, part.ContentID)
		}
	}
	if len(orphaned) > 0 {
		return fmt.Errorf("inline MIME parts have no <img> reference in the HTML body and will appear as unexpected attachments: orphaned cids %v; if you used set_body, make sure the new body preserves all existing cid:... references", orphaned)
	}
	return nil
}
