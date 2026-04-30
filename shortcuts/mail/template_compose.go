// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
	"github.com/larksuite/cli/shortcuts/mail/filecheck"
)

// stdBase64Enc is a local alias used by the template large-attachment
// header encoder. Keeping it here avoids repeated base64 package lookups
// in hot paths and mirrors the draft package's header handling.
var stdBase64Enc = base64.StdEncoding

// Template attachment_type values, matching v1_data_type.Attachment.attachment_type
// (an IDL i32 enum):
//   - 1 (attachmentTypeSmall): embedded in the EML at send time (base64,
//     counted against the 25 MB limit).
//   - 2 (attachmentTypeLarge): uploaded separately; download URL rendered by
//     the server.
//
// Constants are declared in helpers.go and reused here.

// logTemplateInfo emits a structured "info" line to stderr for template
// shortcuts, matching the existing "tip: ... " / "warning: ... " style used
// elsewhere in this package. Callers pass key=value pairs; sensitive fields
// (template_content / subject / recipient plaintext / file_key plaintext)
// must NOT be passed — only counts, flags, and opaque ids.
func logTemplateInfo(runtime *common.RuntimeContext, phase string, fields map[string]interface{}) {
	if runtime == nil {
		return
	}
	out := runtime.IO().ErrOut
	if out == nil {
		return
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	// Stable key order so log lines are diff-friendly.
	sortStrings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, fields[k]))
	}
	fmt.Fprintf(out, "info: template %s: %s\n", phase, strings.Join(parts, " "))
}

func sortStrings(s []string) {
	// tiny insertion sort to avoid importing sort in hot template path.
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// countAddresses returns the recipient count implied by a comma-separated
// address list. Used for key log fields (tos_count/ccs_count/bccs_count).
func countAddresses(raw string) int {
	return len(ParseMailboxList(raw))
}

// countAttachmentsByType returns (inline, large) counts from a template
// attachment slice. Small non-inline entries are derivable as
// len(atts)-inline-large.
func countAttachmentsByType(atts []templateAttachment) (inlineCount, largeCount int) {
	for _, a := range atts {
		if a.IsInline {
			inlineCount++
		}
		if a.AttachmentType == attachmentTypeLarge {
			largeCount++
		}
	}
	return
}

// templateEMLBaseOverhead is the estimated byte cost of template headers and
// address/subject/content envelope when projecting the EML size for LARGE
// attachment switching. Matches desktop's TemplateData base overhead.
const templateEMLBaseOverhead = 2048

// templateLargeSwitchThreshold is the projected EML size (base64) above which
// subsequent template attachments are marked LARGE. Matches the EML 25 MB
// limit used elsewhere and desktop's SMALL_ATTACHMENT_MAX_SIZE.
const templateLargeSwitchThreshold int64 = 25 * 1024 * 1024

// Template-level size limits.
//
//	maxTemplateContentBytes: template_content (HTML body) hard cap, 3 MB.
//	  Matches backend validateTemplateContentSize (open-access/biz/mailtemplate/
//	  template_service.go:1064).
//	maxTemplateBodyInlineSmallBytes: raw-byte ceiling on template_content +
//	  inline image bytes + SMALL non-inline attachment bytes, 25 MB — aligned
//	  with the draft/send EML SMALL-attachment limit so a template that just
//	  barely fits can be applied to a draft without any entry being promoted
//	  at send time. LARGE attachments live on Drive as separate references
//	  and are excluded. When a non-inline attachment would push the running
//	  total over this cap, the builder flips it to LARGE so the rest of the
//	  template still fits; if inline bytes alone already overflow (LARGE is
//	  not an option for inline images — see append()), the builder surfaces
//	  an error.
const (
	maxTemplateContentBytes         int64 = 3 * 1024 * 1024
	maxTemplateBodyInlineSmallBytes int64 = 25 * 1024 * 1024
)

// templateAttachment is the OAPI Attachment payload used in the templates
// create/update request body. Fields align with
// mail.open.access.v1_data_type.Attachment (id/filename/cid/is_inline/
// attachment_type/body).
//
// `body` is a required field on the server (omitting it yields errno 99992402
// `template.attachments[*].body is required`). For files the CLI has already
// uploaded to Drive we reuse the Drive file_key as the body value — the
// backend handler treats both `id` and `body` as the same file_key reference,
// so sending the key twice satisfies the required-field check without forcing
// CLI to stream the raw bytes for every inline image / attachment.
type templateAttachment struct {
	ID             string `json:"id,omitempty"` // Drive file_key
	Filename       string `json:"filename,omitempty"`
	CID            string `json:"cid,omitempty"` // only for is_inline=true
	IsInline       bool   `json:"is_inline"`
	AttachmentType int    `json:"attachment_type,omitempty"` // i32 enum: 1=SMALL, 2=LARGE
	Body           string `json:"body"`                      // required: Drive file_key (same as ID) for uploaded content
}

// templatePayload is the Template struct sent to templates.create / update.
// Field names match the spec's snake_case and the note that to/cc/bcc use
// the plural "tos/ccs/bccs" forms.
type templatePayload struct {
	TemplateID      string               `json:"template_id,omitempty"`
	Name            string               `json:"name"`
	Subject         string               `json:"subject,omitempty"`
	TemplateContent string               `json:"template_content,omitempty"`
	IsPlainTextMode bool                 `json:"is_plain_text_mode"`
	Tos             []templateMailAddr   `json:"tos,omitempty"`
	Ccs             []templateMailAddr   `json:"ccs,omitempty"`
	Bccs            []templateMailAddr   `json:"bccs,omitempty"`
	Attachments     []templateAttachment `json:"attachments,omitempty"`
	CreateTime      string               `json:"create_time,omitempty"`
}

// templateMailAddr matches v1_data_type.MailAddress; on the wire only
// mail_address and (optional) name are emitted. No alias fallback is performed.
type templateMailAddr struct {
	Address string `json:"mail_address"`
	Name    string `json:"name,omitempty"`
}

// parsedLocalImage represents one local file reference discovered in the
// template HTML content. Order is preserved in the order of appearance.
type parsedLocalImage struct {
	RawSrc string // original src attribute value
	Path   string // same as RawSrc; kept for clarity
}

// templateImgSrcRegexp mirrors draftpkg.imgSrcRegexp (unexported). Duplicated
// here because ResolveLocalImagePaths is a sibling helper and this regex is
// private to that package.
var templateImgSrcRegexp = regexp.MustCompile(`(?i)<img\s(?:[^>]*?\s)?src\s*=\s*["']([^"']+)["']`)
var templateURISchemeRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*:`)

// parseLocalImgs extracts local-file <img src="..."> references from HTML, in
// document order. Duplicates are preserved to keep the iteration order
// stable; callers that want dedup by path must do so themselves.
func parseLocalImgs(html string) []parsedLocalImage {
	matches := templateImgSrcRegexp.FindAllStringSubmatch(html, -1)
	var out []parsedLocalImage
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		src := strings.TrimSpace(m[1])
		if src == "" {
			continue
		}
		if strings.HasPrefix(src, "//") {
			continue
		}
		if templateURISchemeRegexp.MatchString(src) {
			continue
		}
		out = append(out, parsedLocalImage{RawSrc: src, Path: src})
	}
	return out
}

// templateMailboxPath builds /open-apis/mail/v1/user_mailboxes/:id/templates[/...].
func templateMailboxPath(mailboxID string, segments ...string) string {
	parts := []string{url.PathEscape(mailboxID), "templates"}
	for _, s := range segments {
		if s == "" {
			continue
		}
		parts = append(parts, url.PathEscape(s))
	}
	return "/open-apis/mail/v1/user_mailboxes/" + strings.Join(parts, "/")
}

// validateTemplateID enforces "decimal integer string" per the spec.
func validateTemplateID(tid string) error {
	if tid == "" {
		return nil
	}
	if _, err := strconv.ParseInt(tid, 10, 64); err != nil {
		return output.ErrValidation("--template-id must be a decimal integer string")
	}
	return nil
}

// renderTemplateAddresses converts a comma-separated address list to
// []templateMailAddr. Empty input returns nil so the field is omitted.
func renderTemplateAddresses(raw string) []templateMailAddr {
	boxes := ParseMailboxList(raw)
	if len(boxes) == 0 {
		return nil
	}
	out := make([]templateMailAddr, 0, len(boxes))
	for _, m := range boxes {
		out = append(out, templateMailAddr{Address: m.Email, Name: m.Name})
	}
	return out
}

// joinTemplateAddresses flattens a []templateMailAddr back to the
// comma-separated "Name <email>" form used by compose helpers.
func joinTemplateAddresses(addrs []templateMailAddr) string {
	if len(addrs) == 0 {
		return ""
	}
	var parts []string
	for _, a := range addrs {
		if a.Address == "" {
			continue
		}
		m := Mailbox{Name: a.Name, Email: a.Address}
		parts = append(parts, m.String())
	}
	return strings.Join(parts, ", ")
}

// generateTemplateCID returns a UUID v4 for inline image Content-IDs.
// Matches draftpkg.generateCID behavior; duplicated only because that
// helper is unexported.
func generateTemplateCID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("failed to generate CID: %w", err)
	}
	return id.String(), nil
}

// uploadToDriveForTemplate uploads a local file to Drive and returns its
// file_key. Files ≤20MB use medias/upload_all; larger files use the
// upload_prepare+upload_part+upload_finish multipart path. parent_type is
// "email" to match the existing large attachment path.
func uploadToDriveForTemplate(ctx context.Context, runtime *common.RuntimeContext, path string) (fileKey string, size int64, err error) {
	info, err := runtime.FileIO().Stat(path)
	if err != nil {
		return "", 0, fmt.Errorf("failed to stat %s: %w", path, err)
	}
	size = info.Size()
	if size > MaxLargeAttachmentSize {
		return "", size, fmt.Errorf("attachment %s (%.1f GB) exceeds the %.0f GB single file limit",
			filepath.Base(path), float64(size)/1024/1024/1024, float64(MaxLargeAttachmentSize)/1024/1024/1024)
	}
	name := filepath.Base(path)
	if err := filecheck.CheckBlockedExtension(name); err != nil {
		return "", size, err
	}
	userOpenId := runtime.UserOpenId()
	if userOpenId == "" {
		return "", size, fmt.Errorf("template attachment upload requires user identity (--as user)")
	}
	if size <= common.MaxDriveMediaUploadSinglePartSize {
		fileKey, err = common.UploadDriveMediaAll(runtime, common.DriveMediaUploadAllConfig{
			FilePath:   path,
			FileName:   name,
			FileSize:   size,
			ParentType: "email",
			ParentNode: &userOpenId,
		})
	} else {
		fileKey, err = common.UploadDriveMediaMultipart(runtime, common.DriveMediaMultipartUploadConfig{
			FilePath:   path,
			FileName:   name,
			FileSize:   size,
			ParentType: "email",
			ParentNode: userOpenId,
		})
	}
	if err != nil {
		return "", size, fmt.Errorf("upload %s to Drive failed: %w", name, err)
	}
	return fileKey, size, nil
}

// templateAttachmentBuilder accumulates attachments while classifying each
// entry SMALL / LARGE according to the projected EML size. Used by both
// +template-create and +template-update so the LARGE-switch decision is
// applied consistently across inline and non-inline entries.
//
// Two independent size ledgers run in parallel:
//   - projectedSize (base64 EML projection) drives the 25 MB send-time
//     LARGE switch (templateLargeSwitchThreshold).
//   - rawBodyInlineSmall (body + inline + SMALL raw bytes) drives the 25 MB
//     template-level cap (maxTemplateBodyInlineSmallBytes). LARGE attachments
//     are excluded because they live on Drive and are fetched by URL, not
//     embedded.
type templateAttachmentBuilder struct {
	projectedSize      int64
	rawBodyInlineSmall int64
	largeBucket        bool
	attachments        []templateAttachment
}

func newTemplateAttachmentBuilder(name, subject, content string, tos, ccs, bccs []templateMailAddr) *templateAttachmentBuilder {
	size := int64(templateEMLBaseOverhead)
	// 4/3 base64 overhead for the raw fields.
	bytesEncoded := int64(len(name)+len(subject)+len(content))*4/3 + int64(200)
	size += bytesEncoded
	for _, a := range tos {
		size += int64(len(a.Address) + len(a.Name) + 16)
	}
	for _, a := range ccs {
		size += int64(len(a.Address) + len(a.Name) + 16)
	}
	for _, a := range bccs {
		size += int64(len(a.Address) + len(a.Name) + 16)
	}
	return &templateAttachmentBuilder{
		projectedSize:      size,
		rawBodyInlineSmall: int64(len(content)),
	}
}

// append adds one attachment, picking SMALL or LARGE for non-inline entries
// based on the projected EML size running total and the 25 MB body+inline+
// SMALL cap. Once largeBucket flips to true, every subsequent non-inline
// attachment is LARGE regardless of size. Inline images are always SMALL:
// they are referenced from the HTML body via cid:<id> and therefore must be
// embedded in the MIME parts of the EML; the LARGE flavor (server-rendered
// download URL) would break the <img src> reference in every mail client.
func (b *templateAttachmentBuilder) append(fileKey, filename, cid string, isInline bool, fileSize int64) {
	base64Size := estimateBase64EMLSize(fileSize)
	aType := attachmentTypeSmall
	if isInline {
		// Inline images cannot be LARGE; still fold their base64 footprint
		// into projectedSize so any subsequent non-inline attachment sees
		// the correct cumulative EML size and flips to LARGE when needed.
		// Raw bytes also count toward the 25 MB body+inline+SMALL cap; if
		// inline alone overflows, finalize() will surface an error because
		// we cannot bump inline to LARGE.
		b.projectedSize += base64Size
		b.rawBodyInlineSmall += fileSize
	} else if b.largeBucket ||
		b.projectedSize+base64Size >= templateLargeSwitchThreshold ||
		b.rawBodyInlineSmall+fileSize > maxTemplateBodyInlineSmallBytes {
		// Non-inline that would overflow either ledger → LARGE. LARGE is
		// excluded from both totals (served by Drive URL, not in the EML).
		aType = attachmentTypeLarge
		b.largeBucket = true
	} else {
		b.projectedSize += base64Size
		b.rawBodyInlineSmall += fileSize
	}
	b.attachments = append(b.attachments, templateAttachment{
		ID:             fileKey,
		Filename:       filename,
		CID:            cid,
		IsInline:       isInline,
		AttachmentType: aType,
		// The server marks `body` as required (errno 99992402). Since the
		// file was already uploaded to Drive and the handler resolves
		// Attachment.id as the file_key, mirror the same key into body so
		// the required-field check passes without the CLI re-reading the
		// file bytes.
		Body: fileKey,
	})
}

// finalize runs after all attachments have been appended, validating the
// 25 MB template-level ceiling on body+inline+SMALL raw bytes. The cap only
// fires when inline images alone overflow it; non-inline overflow is
// self-healing via the LARGE switch inside append().
func (b *templateAttachmentBuilder) finalize() error {
	if b.rawBodyInlineSmall > maxTemplateBodyInlineSmallBytes {
		return fmt.Errorf("template body + inline images exceed %d MB (got %.1f MB); "+
			"reduce inline image size or count — inline images cannot be promoted to LARGE",
			maxTemplateBodyInlineSmallBytes/(1024*1024),
			float64(b.rawBodyInlineSmall)/1024/1024)
	}
	return nil
}

// wrapTemplateContentIfNeeded mirrors the draft compose flow's plain-text →
// HTML upgrade (shortcuts/mail/mail_quote.go:buildBodyDiv): HTML-escape the
// content and convert newlines to <br> so the PC client renders line breaks
// in template preview. Without this, a three-line plain body saved verbatim
// renders as a single run-on line because HTML collapses whitespace. The
// transform is applied for both is_plain_text_mode=true and =false; the
// preview always renders the stored content as HTML, and the send path
// reads is_plain_text_mode separately to decide whether to strip back to
// plain text (see mergeTemplateBody).
func wrapTemplateContentIfNeeded(content string, isPlainText bool) string {
	if content == "" {
		return content
	}
	if bodyIsHTML(content) {
		return content
	}
	return buildBodyDiv(content, false)
}

// buildTemplatePayloadFromFlags processes HTML inline images and non-inline
// attachment flags in the exact order required by the spec: inline images in
// HTML <img> order, non-inline attachments in --attach / --attachment
// flag order. Returns the rewritten template content (cid: refs) plus the
// attachment list.
func buildTemplatePayloadFromFlags(
	ctx context.Context,
	runtime *common.RuntimeContext,
	name, subject, content string,
	tos, ccs, bccs []templateMailAddr,
	attachPaths []string,
) (rewrittenContent string, atts []templateAttachment, err error) {
	builder := newTemplateAttachmentBuilder(name, subject, content, tos, ccs, bccs)

	// 1. Inline images (iterate in the HTML order so cid mapping is stable
	// across CLI versions; duplicates reuse the same file_key/cid).
	imgs := parseLocalImgs(content)
	pathToCID := make(map[string]string)
	pathToFileKey := make(map[string]string)
	pathToSize := make(map[string]int64)
	for _, img := range imgs {
		if cid, ok := pathToCID[img.Path]; ok {
			// Re-write the next occurrence to the same cid.
			content = replaceImgSrcOnce(content, img.RawSrc, "cid:"+cid)
			continue
		}
		fileKey, sz, upErr := uploadToDriveForTemplate(ctx, runtime, img.Path)
		if upErr != nil {
			return "", nil, upErr
		}
		cid, cidErr := generateTemplateCID()
		if cidErr != nil {
			return "", nil, cidErr
		}
		pathToCID[img.Path] = cid
		pathToFileKey[img.Path] = fileKey
		pathToSize[img.Path] = sz
		content = replaceImgSrcOnce(content, img.RawSrc, "cid:"+cid)
		builder.append(fileKey, filepath.Base(img.Path), cid, true, sz)
	}

	// 2. Non-inline --attach paths in the exact order passed.
	for _, p := range attachPaths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		fileKey, sz, upErr := uploadToDriveForTemplate(ctx, runtime, p)
		if upErr != nil {
			return "", nil, upErr
		}
		builder.append(fileKey, filepath.Base(p), "", false, sz)
	}

	if err := builder.finalize(); err != nil {
		return "", nil, err
	}
	return content, builder.attachments, nil
}

// replaceImgSrcOnce rewrites the first <img src="rawSrc"> occurrence to
// <img src="newSrc">, preserving the quoting style of the original.
func replaceImgSrcOnce(html, rawSrc, newSrc string) string {
	// Find the next <img ...> match whose captured src equals rawSrc.
	indices := templateImgSrcRegexp.FindAllStringSubmatchIndex(html, -1)
	for _, idx := range indices {
		if len(idx) < 4 {
			continue
		}
		if strings.TrimSpace(html[idx[2]:idx[3]]) == rawSrc {
			return html[:idx[2]] + newSrc + html[idx[3]:]
		}
	}
	return html
}

// ── Template fetch / CRUD ────────────────────────────────────────────

// fetchTemplate GETs a single template (full fields) for --template-id
// composition and update patch workflows.
func fetchTemplate(runtime *common.RuntimeContext, mailboxID, templateID string) (*templatePayload, error) {
	data, err := runtime.CallAPI("GET", templateMailboxPath(mailboxID, templateID), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch template %s failed: %w", templateID, err)
	}
	return extractTemplatePayload(data)
}

// extractTemplatePayload decodes the API response, looking inside the common
// "template" wrapper when present.
func extractTemplatePayload(data map[string]interface{}) (*templatePayload, error) {
	raw := data
	if t, ok := data["template"].(map[string]interface{}); ok {
		raw = t
	}
	if raw == nil {
		return nil, fmt.Errorf("API response missing template body")
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("re-encode template payload failed: %w", err)
	}
	var out templatePayload
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("decode template payload failed: %w", err)
	}
	return &out, nil
}

// createTemplate POSTs a new template.
func createTemplate(runtime *common.RuntimeContext, mailboxID string, tpl *templatePayload) (map[string]interface{}, error) {
	return runtime.CallAPI("POST", templateMailboxPath(mailboxID), nil, map[string]interface{}{
		"template": tpl,
	})
}

// updateTemplate PUTs a full-replace update.
func updateTemplate(runtime *common.RuntimeContext, mailboxID, templateID string, tpl *templatePayload) (map[string]interface{}, error) {
	return runtime.CallAPI("PUT", templateMailboxPath(mailboxID, templateID), nil, map[string]interface{}{
		"template": tpl,
	})
}

// ── --template-id merge logic (§5.5) ─────────────────────────────────

// templateInlineRef describes one inline image carried by an applied
// template. Callers download the bytes from Drive (via FileKey) and
// register the CID with the EML builder's inline parts so the HTML body's
// <img src="cid:..."> references resolve against a real MIME part.
type templateInlineRef struct {
	FileKey  string
	Filename string
	CID      string
}

// templateAttachmentRef describes one SMALL non-inline attachment carried
// by an applied template. These are regular file attachments (not inline
// images), so they have no CID. Callers fetch the bytes via
// embedTemplateSmallAttachments and register them on the EML builder as
// plain MIME attachment parts.
type templateAttachmentRef struct {
	FileKey  string
	Filename string
}

// templateApplyResult holds the merged compose state produced by
// applyTemplate. Callers consume individual fields and feed them into the
// existing +send / +reply / +forward pipelines.
type templateApplyResult struct {
	To              string
	Cc              string
	Bcc             string
	Subject         string
	Body            string
	IsPlainTextMode bool
	Warnings        []string
	// LargeAttachmentIDs carries Drive file_keys for the template's true
	// LARGE (attachment_type=2) non-inline entries. Callers pass these
	// through the X-Lms-Large-Attachment-Ids header so the server renders
	// them as download-link attachments; inline and SMALL entries must
	// NOT be included (they'd be promoted to LARGE, turning embedded
	// content into bare download URLs).
	LargeAttachmentIDs []string
	// InlineAttachments carries template inline images (IsInline=true, SMALL
	// type) whose <img src="cid:..."> references appear in Body. The CLI
	// must fetch each file_key from Drive and register it via
	// emlbuilder.AddInline so the draft compose pipeline's inline CID
	// validation passes and the sent mail renders the image.
	InlineAttachments []templateInlineRef
	// SmallAttachments carries SMALL non-inline template attachments
	// (IsInline=false, attachment_type=1). The CLI fetches each file_key's
	// bytes via the template attachments/download_url API and registers
	// them through emlbuilder.AddAttachment so they end up embedded in the
	// EML (matching draft-compose behavior for regular attachments).
	SmallAttachments []templateAttachmentRef
}

// templateShortcutKind enumerates the 5 shortcuts that accept --template-id.
type templateShortcutKind string

const (
	templateShortcutSend        templateShortcutKind = "send"
	templateShortcutDraftCreate templateShortcutKind = "draft-create"
	templateShortcutReply       templateShortcutKind = "reply"
	templateShortcutReplyAll    templateShortcutKind = "reply-all"
	templateShortcutForward     templateShortcutKind = "forward"
)

// applyTemplate merges a fetched template with draft-derived and user-flag
// values. draftTo/Cc/Bcc are the addresses already on the draft (from the
// original message for reply/reply-all/forward, or the user flags for send/
// draft-create). userTo/Cc/Bcc/Subject/Body are user-supplied flag values
// (empty string = not provided).
func applyTemplate(
	kind templateShortcutKind,
	tpl *templatePayload,
	draftTo, draftCc, draftBcc string,
	draftSubject string,
	draftBody string,
	userTo, userCc, userBcc, userSubject, userBody string,
) templateApplyResult {
	res := templateApplyResult{}

	// Start with whatever is already in the draft (or the user-explicit
	// draft-to values for send/draft-create).
	effTo := draftTo
	effCc := draftCc
	effBcc := draftBcc
	// User-flag --to/--cc/--bcc values override draft-derived values
	// before template injection.
	if userTo != "" {
		effTo = userTo
	}
	if userCc != "" {
		effCc = userCc
	}
	if userBcc != "" {
		effBcc = userBcc
	}

	tplTo := joinTemplateAddresses(tpl.Tos)
	tplCc := joinTemplateAddresses(tpl.Ccs)
	tplBcc := joinTemplateAddresses(tpl.Bccs)

	// Append template to/cc/bcc into draft to/cc/bcc.
	effTo = appendAddrList(effTo, tplTo)
	effCc = appendAddrList(effCc, tplCc)
	effBcc = appendAddrList(effBcc, tplBcc)

	res.To = effTo
	res.Cc = effCc
	res.Bcc = effBcc

	// Q2: subject merging. User --subject wins, else draft non-empty wins,
	// else template subject.
	switch {
	case strings.TrimSpace(userSubject) != "":
		res.Subject = userSubject
	case strings.TrimSpace(draftSubject) != "":
		res.Subject = draftSubject
	default:
		res.Subject = tpl.Subject
	}

	// Q3: body merging. The shortcut-specific HTML/plain-text injection is
	// handled by the caller; applyTemplate returns a merged body string that
	// the caller can feed back into its compose pipeline.
	res.Body = mergeTemplateBody(kind, tpl, draftBody, userBody)

	// IsPlainTextMode propagation: template value wins.
	res.IsPlainTextMode = tpl.IsPlainTextMode

	// Q4: warn when reply / reply-all + template has to/cc/bcc (likely
	// duplicates against the reply-derived recipients).
	if (kind == templateShortcutReply || kind == templateShortcutReplyAll) &&
		(len(tpl.Tos) > 0 || len(tpl.Ccs) > 0 || len(tpl.Bccs) > 0) {
		res.Warnings = append(res.Warnings,
			"template to/cc/bcc are appended without de-duplication; "+
				"you may see repeated recipients. Use --to/--cc/--bcc to override, "+
				"or run +template-update to clear template addresses.")
	}

	// Classify template attachments by (inline, attachment_type) into the
	// three output channels. Each classification drives a different draft-
	// compose wiring:
	//   inline+SMALL     → embedTemplateInlineAttachments (AddInline, CID-bound)
	//   non-inline+SMALL → embedTemplateSmallAttachments (AddAttachment)
	//   non-inline+LARGE → X-Lms-Large-Attachment-Ids header (server renders URL)
	// Anomalous combinations (inline+LARGE, inline without CID) are dropped
	// with a warning because they cannot round-trip through any of the three
	// pipelines.
	for _, att := range tpl.Attachments {
		if att.ID == "" {
			continue
		}
		if att.IsInline {
			if att.CID == "" {
				res.Warnings = append(res.Warnings,
					fmt.Sprintf("template inline attachment %q has no cid; skipping (HTML body cannot reference it)", att.Filename))
				continue
			}
			if att.AttachmentType == attachmentTypeLarge {
				res.Warnings = append(res.Warnings,
					fmt.Sprintf("template inline attachment %q is marked LARGE; skipping (inline images must be SMALL to embed in the EML)", att.Filename))
				continue
			}
			res.InlineAttachments = append(res.InlineAttachments, templateInlineRef{
				FileKey:  att.ID,
				Filename: att.Filename,
				CID:      att.CID,
			})
			continue
		}
		// Non-inline: SMALL → embedded as a regular attachment part, LARGE →
		// download-URL header.
		if att.AttachmentType == attachmentTypeLarge {
			res.LargeAttachmentIDs = append(res.LargeAttachmentIDs, att.ID)
		} else {
			res.SmallAttachments = append(res.SmallAttachments, templateAttachmentRef{
				FileKey:  att.ID,
				Filename: att.Filename,
			})
		}
	}

	return res
}

func appendAddrList(base, extra string) string {
	if strings.TrimSpace(extra) == "" {
		return base
	}
	if strings.TrimSpace(base) == "" {
		return extra
	}
	// §5.5 Q1 is explicit: concat without dedup.
	return base + ", " + extra
}

// mergeTemplateBody handles §5.5 Q3 body merging.
//
//   - send / draft-create: empty draft body → use template body; non-empty →
//     append template body after a separator.
//   - reply / reply-all / forward: insert template body before the
//     <blockquote> wrapper (regex), fallback to end-append; plain-text drafts
//     prepend template body + newline before the quoted block.
func mergeTemplateBody(kind templateShortcutKind, tpl *templatePayload, draftBody, userBody string) string {
	tplContent := tpl.TemplateContent
	// If the user explicitly passed --body, that is the composer's own
	// authoring area; we still inject the template content into the same
	// area (draft_body = user_body for send/draft-create).
	if userBody != "" {
		draftBody = userBody
	}

	// Plain-text-mode templates store content as HTML (so the preview shows
	// line breaks) but the email body must be sent as plain text. Reverse
	// the wrapping here using stripHTMLForQuote (which already converts
	// <br>/</div> into \n and unescapes entities) so the recipient sees
	// real newlines instead of literal <div>...</div> markup. Templates
	// authored via the Lark client use the same HTML-wrapped storage, so
	// this also fixes apply for client-authored plain-text templates.
	if tpl.IsPlainTextMode && bodyIsHTML(tplContent) {
		tplContent = stripHTMLForQuote(tplContent)
	}

	// Plain-text mode: simple append.
	if tpl.IsPlainTextMode {
		switch kind {
		case templateShortcutSend, templateShortcutDraftCreate:
			if strings.TrimSpace(draftBody) == "" {
				return tplContent
			}
			return draftBody + "\n\n" + tplContent
		default:
			// reply/forward plain-text: prepend template before quote.
			// emlbuilder composes quote separately so the draft body here
			// is pure user-authored content; we just prepend.
			if strings.TrimSpace(draftBody) == "" {
				return tplContent
			}
			return tplContent + "\n\n" + draftBody
		}
	}

	switch kind {
	case templateShortcutSend, templateShortcutDraftCreate:
		if strings.TrimSpace(draftBody) == "" {
			return tplContent
		}
		// Match the plain-text branch's explicit separator so template
		// markup doesn't butt up against user-authored HTML.
		return draftBody + "<br><br>" + tplContent
	case templateShortcutReply, templateShortcutReplyAll, templateShortcutForward:
		// At this compose layer, draftBody is the user-authored area only
		// (the caller adds the quote block downstream). Inject template
		// content at the head of that area so it lands above the future
		// quote block.
		if strings.TrimSpace(draftBody) == "" {
			return tplContent
		}
		// Regex replace: if the draft body already contains a quote block
		// (some callers pre-compose it), insert template before it.
		if draftpkg.HTMLContainsLargeAttachment(draftBody) {
			// fall through — no quote heuristic; appending is safe.
		}
		merged := draftpkg.InsertBeforeQuoteOrAppend(draftBody, tplContent)
		if merged != draftBody {
			return merged
		}
		return tplContent + draftBody
	}
	return draftBody
}

// encodeTemplateLargeAttachmentHeader returns the base64-JSON-encoded value
// to add to an X-Lms-Large-Attachment-Ids header when the template supplies
// one or more non-inline file_keys. Returns empty string when the input is
// empty (caller should skip adding the header).
//
// Duplicate IDs are collapsed into a single entry.
func encodeTemplateLargeAttachmentHeader(tplIDs []string) (string, error) {
	if len(tplIDs) == 0 {
		return "", nil
	}
	seen := make(map[string]bool, len(tplIDs))
	var deduped []largeAttID
	for _, id := range tplIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		deduped = append(deduped, largeAttID{ID: id})
	}
	if len(deduped) == 0 {
		return "", nil
	}
	buf, err := json.Marshal(deduped)
	if err != nil {
		return "", err
	}
	return b64StdEncode(buf), nil
}

// b64StdEncode avoids importing encoding/base64 twice.
func b64StdEncode(buf []byte) string { return stdBase64Enc.EncodeToString(buf) }

// fetchTemplateAttachmentURLs resolves time-limited download URLs for a
// batch of template attachment IDs via the
// user_mailbox.template.attachments.download_url API. Returns a map from
// attachment_id to signed download URL. Failed IDs surface as warnings in
// the returned list so the caller can decide whether to abort.
//
// The endpoint accepts up to N attachment_ids per call; we batch at 20 to
// stay well under the query-string limit.
func fetchTemplateAttachmentURLs(
	runtime *common.RuntimeContext,
	mailboxID, templateID string,
	attachmentIDs []string,
) (map[string]string, []warningEntry, error) {
	if len(attachmentIDs) == 0 {
		return nil, nil, nil
	}
	urlMap := make(map[string]string, len(attachmentIDs))
	warnings := make([]warningEntry, 0)
	const batchSize = 20
	for i := 0; i < len(attachmentIDs); i += batchSize {
		end := i + batchSize
		if end > len(attachmentIDs) {
			end = len(attachmentIDs)
		}
		batch := attachmentIDs[i:end]

		parts := make([]string, len(batch))
		for j, id := range batch {
			parts[j] = "attachment_ids=" + url.QueryEscape(id)
		}
		apiURL := templateMailboxPath(mailboxID, templateID) + "/attachments/download_url?" + strings.Join(parts, "&")

		data, err := runtime.CallAPI("GET", apiURL, nil, nil)
		if err != nil {
			return nil, warnings, fmt.Errorf("template attachments/download_url (template_id=%s): %w", templateID, err)
		}
		if urls, ok := data["download_urls"].([]interface{}); ok {
			for _, item := range urls {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				attID := strVal(m["attachment_id"])
				dlURL := strVal(m["download_url"])
				if attID != "" && dlURL != "" {
					urlMap[attID] = dlURL
				}
			}
		}
		// The template variant of the endpoint surfaces failures under
		// "failed_reasons" (see registry meta mail.json:5614-5632). Record
		// each as a warning so callers can log and skip.
		if failed, ok := data["failed_reasons"].([]interface{}); ok {
			for _, item := range failed {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				warnings = append(warnings, warningEntry{
					Code:         "template_attachment_download_url_failed",
					Level:        "warning",
					AttachmentID: strVal(m["attachment_id"]),
					Detail:       strVal(m["reason"]),
					Retryable:    false,
				})
			}
		}
	}
	return urlMap, warnings, nil
}

// embedTemplateInlineAttachments batch-resolves the template inline image
// download URLs via user_mailbox.template.attachments.download_url, fetches
// each pre-signed URL's bytes, and registers them with the EML builder as
// CID-referenced inline parts. Returns the augmented builder plus the list
// of CIDs registered so the caller can extend its allCIDs set before
// validateInlineCIDs. Entries whose CID is not referenced in the HTML body
// (e.g. body was edited without pruning the attachment list) are silently
// skipped to avoid unreferenced-MIME-part bloat.
func embedTemplateInlineAttachments(
	ctx context.Context,
	runtime *common.RuntimeContext,
	bld emlbuilder.Builder,
	htmlBody string,
	mailboxID, templateID string,
	refs []templateInlineRef,
) (emlbuilder.Builder, []string, error) {
	if len(refs) == 0 || templateID == "" {
		return bld, nil, nil
	}
	// Filter to refs actually referenced in the HTML body.
	wanted := make([]templateInlineRef, 0, len(refs))
	for _, ref := range refs {
		if ref.CID == "" || ref.FileKey == "" {
			continue
		}
		if !strings.Contains(htmlBody, "cid:"+ref.CID) {
			continue
		}
		wanted = append(wanted, ref)
	}
	if len(wanted) == 0 {
		return bld, nil, nil
	}
	ids := make([]string, 0, len(wanted))
	for _, ref := range wanted {
		ids = append(ids, ref.FileKey)
	}
	urlMap, warns, err := fetchTemplateAttachmentURLs(runtime, mailboxID, templateID, ids)
	if err != nil {
		return bld, nil, err
	}
	for _, w := range warns {
		fmt.Fprintf(runtime.IO().ErrOut, "warning: code=%s attachment_id=%s detail=%s\n", w.Code, w.AttachmentID, w.Detail)
	}
	registered := make([]string, 0, len(wanted))
	for _, ref := range wanted {
		dlURL, ok := urlMap[ref.FileKey]
		if !ok || dlURL == "" {
			return bld, nil, fmt.Errorf("template inline image %q (cid=%s): download URL not returned by server", ref.Filename, ref.CID)
		}
		bytes, err := downloadAttachmentContent(runtime, dlURL)
		if err != nil {
			return bld, nil, fmt.Errorf("template inline image %q (cid=%s): %w", ref.Filename, ref.CID, err)
		}
		filename := ref.Filename
		if filename == "" {
			filename = ref.CID
		}
		contentType, err := filecheck.CheckInlineImageFormat(filename, bytes)
		if err != nil {
			return bld, nil, fmt.Errorf("template inline image %q (cid=%s): %w", filename, ref.CID, err)
		}
		bld = bld.AddInline(bytes, contentType, filename, ref.CID)
		registered = append(registered, ref.CID)
	}
	return bld, registered, nil
}

// embedTemplateSmallAttachments batch-resolves the template SMALL non-inline
// attachment download URLs via user_mailbox.template.attachments.download_url,
// fetches each pre-signed URL's bytes, and registers them with the EML
// builder via AddAttachment (matching the content-type canonicalization
// that AddFileAttachment uses: application/octet-stream). Returns the
// augmented builder plus the total raw bytes so the caller can feed
// them into the EML size budget.
func embedTemplateSmallAttachments(
	ctx context.Context,
	runtime *common.RuntimeContext,
	bld emlbuilder.Builder,
	mailboxID, templateID string,
	refs []templateAttachmentRef,
) (emlbuilder.Builder, int64, error) {
	if len(refs) == 0 || templateID == "" {
		return bld, 0, nil
	}
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.FileKey == "" {
			continue
		}
		ids = append(ids, ref.FileKey)
	}
	if len(ids) == 0 {
		return bld, 0, nil
	}
	urlMap, warns, err := fetchTemplateAttachmentURLs(runtime, mailboxID, templateID, ids)
	if err != nil {
		return bld, 0, err
	}
	for _, w := range warns {
		fmt.Fprintf(runtime.IO().ErrOut, "warning: code=%s attachment_id=%s detail=%s\n", w.Code, w.AttachmentID, w.Detail)
	}
	var totalBytes int64
	for _, ref := range refs {
		if ref.FileKey == "" {
			continue
		}
		dlURL, ok := urlMap[ref.FileKey]
		if !ok || dlURL == "" {
			return bld, 0, fmt.Errorf("template attachment %q: download URL not returned by server", ref.Filename)
		}
		buf, err := downloadAttachmentContent(runtime, dlURL)
		if err != nil {
			return bld, 0, fmt.Errorf("template attachment %q: %w", ref.Filename, err)
		}
		filename := ref.Filename
		if filename == "" {
			filename = ref.FileKey
		}
		// Match AddFileAttachment's backend-aligned content-type: regular
		// attachments are canonicalized to application/octet-stream on
		// save/readback, so the builder should emit the same.
		bld = bld.AddAttachment(buf, "application/octet-stream", filename)
		totalBytes += int64(len(buf))
	}
	return bld, totalBytes, nil
}
