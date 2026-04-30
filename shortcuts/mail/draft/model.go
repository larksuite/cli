// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/mail"
	"strings"
	"time"

	"github.com/larksuite/cli/extension/fileio"
)

type DraftRaw struct {
	DraftID string
	RawEML  string
}

type DraftResult struct {
	DraftID   string
	Reference string
}

type Header struct {
	Name  string
	Value string
}

type Address struct {
	Name    string `json:"name,omitempty"`
	Address string `json:"address"`
}

func (a Address) String() string {
	if a.Name == "" {
		return a.Address
	}
	return (&mail.Address{Name: a.Name, Address: a.Address}).String()
}

type Part struct {
	PartID string

	Headers               []Header
	MediaType             string
	MediaParams           map[string]string
	ContentDisposition    string
	ContentDispositionArg map[string]string
	ContentID             string
	TransferEncoding      string

	Children []*Part
	Body     []byte

	Preamble []byte
	Epilogue []byte

	RawEntity []byte
	Dirty     bool

	// EncodingProblem is set when the part's body could not be decoded as
	// declared (e.g. malformed base64, bad charset, unparseable Content-Type).
	// The part still contains usable data (raw bytes or fallback decode) and
	// can round-trip through RawEntity, but callers should treat Body as
	// potentially degraded.
	EncodingProblem bool
}

func (p *Part) IsMultipart() bool {
	return p != nil && strings.HasPrefix(strings.ToLower(p.MediaType), "multipart/")
}

func (p *Part) Clone() *Part {
	if p == nil {
		return nil
	}
	cp := *p
	cp.Headers = append([]Header{}, p.Headers...)
	cp.MediaParams = cloneStringMap(p.MediaParams)
	cp.ContentDispositionArg = cloneStringMap(p.ContentDispositionArg)
	cp.Body = append([]byte{}, p.Body...)
	cp.Preamble = append([]byte{}, p.Preamble...)
	cp.Epilogue = append([]byte{}, p.Epilogue...)
	cp.RawEntity = append([]byte{}, p.RawEntity...)
	cp.Dirty = p.Dirty
	cp.Children = make([]*Part, 0, len(p.Children))
	for _, child := range p.Children {
		cp.Children = append(cp.Children, child.Clone())
	}
	return &cp
}

func (p *Part) FileName() string {
	if p == nil {
		return ""
	}
	if name := p.ContentDispositionArg["filename"]; name != "" {
		return name
	}
	if name := p.MediaParams["name"]; name != "" {
		return name
	}
	return ""
}

// DraftCtx carries runtime dependencies for draft operations.
// It is separate from DraftSnapshot to keep the snapshot a pure data model.
type DraftCtx struct {
	FIO fileio.FileIO
}

type DraftSnapshot struct {
	DraftID string
	Headers []Header
	Body    *Part

	Subject    string
	From       []Address
	To         []Address
	Cc         []Address
	Bcc        []Address
	ReplyTo    []Address
	MessageID  string
	InReplyTo  string
	References string

	PrimaryTextPartID string
	PrimaryHTMLPartID string
}

type PartSummary struct {
	PartID      string `json:"part_id"`
	FileName    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Disposition string `json:"disposition,omitempty"`
	CID         string `json:"cid,omitempty"`
}

// LargeAttachmentSummary describes a single large attachment registered in
// the draft via the X-Lms-Large-Attachment-Ids header. Unlike normal
// attachments, large attachments have no MIME part — their existence is
// conveyed by the header plus an HTML card in the body.
type LargeAttachmentSummary struct {
	Token     string `json:"token"`
	FileName  string `json:"filename,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

type DraftProjection struct {
	Subject                 string                   `json:"subject"`
	To                      []Address                `json:"to,omitempty"`
	Cc                      []Address                `json:"cc,omitempty"`
	Bcc                     []Address                `json:"bcc,omitempty"`
	ReplyTo                 []Address                `json:"reply_to,omitempty"`
	InReplyTo               string                   `json:"in_reply_to,omitempty"`
	References              string                   `json:"references,omitempty"`
	BodyText                string                   `json:"body_text,omitempty"`
	BodyHTMLSummary         string                   `json:"body_html_summary,omitempty"`
	HasQuotedContent        bool                     `json:"has_quoted_content,omitempty"`
	HasSignature            bool                     `json:"has_signature,omitempty"`
	SignatureID             string                   `json:"signature_id,omitempty"`
	AttachmentsSummary      []PartSummary            `json:"attachments_summary,omitempty"`
	LargeAttachmentsSummary []LargeAttachmentSummary `json:"large_attachments_summary,omitempty"`
	InlineSummary           []PartSummary            `json:"inline_summary,omitempty"`
	Warnings                []string                 `json:"warnings,omitempty"`
}

type Patch struct {
	Ops     []PatchOp    `json:"ops"`
	Options PatchOptions `json:"options,omitempty"`
}

type PatchOptions struct {
	RewriteEntireDraft        bool `json:"rewrite_entire_draft,omitempty"`
	AllowProtectedHeaderEdits bool `json:"allow_protected_header_edits,omitempty"`
}

type AttachmentTarget struct {
	PartID string `json:"part_id,omitempty"`
	CID    string `json:"cid,omitempty"`
	// Token selects a large attachment by its file token (registered via
	// the X-Lms-Large-Attachment-Ids header). Only valid for
	// remove_attachment; replace_inline/remove_inline operate on MIME
	// parts and do not accept Token.
	Token string `json:"token,omitempty"`
}

// hasKey reports whether a PartID or CID is set. Used for ops that
// target MIME parts (replace_inline, remove_inline).
func (t AttachmentTarget) hasKey() bool {
	return strings.TrimSpace(t.PartID) != "" || strings.TrimSpace(t.CID) != ""
}

// hasAnyKey reports whether any locator (PartID, CID, or Token) is set.
// Used for remove_attachment which supports all three.
func (t AttachmentTarget) hasAnyKey() bool {
	return t.hasKey() || strings.TrimSpace(t.Token) != ""
}

type PatchOp struct {
	Op          string           `json:"op"`
	Value       string           `json:"value,omitempty"`
	Field       string           `json:"field,omitempty"`
	Address     string           `json:"address,omitempty"`
	Name        string           `json:"name,omitempty"`
	Addresses   []Address        `json:"addresses,omitempty"`
	BodyKind    string           `json:"body_kind,omitempty"`
	Selector    string           `json:"selector,omitempty"`
	Path        string           `json:"path,omitempty"`
	CID         string           `json:"cid,omitempty"`
	FileName    string           `json:"filename,omitempty"`
	ContentType string           `json:"content_type,omitempty"`
	Target      AttachmentTarget `json:"target,omitempty"`
	SignatureID string           `json:"signature_id,omitempty"`

	// Calendar event fields, used by set_calendar. The raw ISO 8601 strings
	// are shown in dry-run output; the shortcut layer pre-builds the ICS
	// blob into CalendarICS below before Apply runs.
	EventSummary  string `json:"event_summary,omitempty"`
	EventStart    string `json:"event_start,omitempty"`
	EventEnd      string `json:"event_end,omitempty"`
	EventLocation string `json:"event_location,omitempty"`

	// RenderedSignatureHTML is set by the shortcut layer (not from JSON) after
	// fetching and interpolating the signature. The patch layer uses this
	// pre-rendered content for insert_signature ops.
	RenderedSignatureHTML string           `json:"-"`
	SignatureImages       []SignatureImage `json:"-"`

	// CalendarICS holds the pre-built RFC 5545 ICS blob for a set_calendar
	// op. Populated by the shortcut layer after the snapshot is parsed and
	// organizer/attendee addresses can be resolved. Not serialised.
	CalendarICS []byte `json:"-"`
}

// SignatureImage holds pre-downloaded image data for signature inline images.
// Populated by the shortcut layer, consumed by the patch layer.
type SignatureImage struct {
	CID         string
	ContentType string
	FileName    string
	Data        []byte
}

func (p Patch) Validate() error {
	if len(p.Ops) == 0 {
		return fmt.Errorf("patch ops is required")
	}
	for i, op := range p.Ops {
		if err := op.Validate(); err != nil {
			return fmt.Errorf("invalid patch op #%d: %w", i+1, err)
		}
	}
	return nil
}

func (op PatchOp) Validate() error {
	switch op.Op {
	case "set_subject":
		if strings.TrimSpace(op.Value) == "" {
			return fmt.Errorf("set_subject requires value")
		}
		if strings.ContainsAny(op.Value, "\r\n") {
			return fmt.Errorf("set_subject: value must not contain CR or LF")
		}
	case "set_recipients":
		if !isRecipientField(op.Field) {
			return fmt.Errorf("recipient field must be one of to/cc/bcc")
		}
		for _, addr := range op.Addresses {
			if strings.TrimSpace(addr.Address) == "" {
				return fmt.Errorf("set_recipients requires non-empty addresses")
			}
		}
	case "add_recipient", "remove_recipient":
		if !isRecipientField(op.Field) {
			return fmt.Errorf("recipient field must be one of to/cc/bcc")
		}
		if strings.TrimSpace(op.Address) == "" {
			return fmt.Errorf("%s requires address", op.Op)
		}
	case "set_reply_to":
		if len(op.Addresses) == 0 {
			return fmt.Errorf("set_reply_to requires addresses")
		}
	case "clear_reply_to":
	case "set_body", "set_reply_body":
	case "replace_body", "append_body":
		if !isBodyKind(op.BodyKind) {
			return fmt.Errorf("body_kind must be text/plain or text/html")
		}
		if op.Selector != "" && op.Selector != "primary" {
			return fmt.Errorf("selector must be primary")
		}
	case "set_header":
		if strings.TrimSpace(op.Name) == "" {
			return fmt.Errorf("set_header requires name")
		}
		if strings.ContainsAny(op.Name, ":\r\n") {
			return fmt.Errorf("set_header: header name must not contain ':', CR, or LF")
		}
		if strings.ContainsAny(op.Value, "\r\n") {
			return fmt.Errorf("set_header: header value must not contain CR or LF")
		}
	case "remove_header":
		if strings.TrimSpace(op.Name) == "" {
			return fmt.Errorf("remove_header requires name")
		}
	case "add_attachment":
		if strings.TrimSpace(op.Path) == "" {
			return fmt.Errorf("add_attachment requires path")
		}
	case "remove_attachment":
		if !op.Target.hasAnyKey() {
			return fmt.Errorf("remove_attachment requires target with at least one of part_id, cid, or token")
		}
	case "add_inline":
		if strings.TrimSpace(op.Path) == "" {
			return fmt.Errorf("add_inline requires path")
		}
		if strings.TrimSpace(op.CID) == "" {
			return fmt.Errorf("add_inline requires cid")
		}
	case "replace_inline":
		if !op.Target.hasKey() {
			return fmt.Errorf("replace_inline requires target with at least one of part_id or cid")
		}
		if strings.TrimSpace(op.Path) == "" {
			return fmt.Errorf("replace_inline requires path")
		}
	case "remove_inline":
		if !op.Target.hasKey() {
			return fmt.Errorf("remove_inline requires target with at least one of part_id or cid")
		}
	case "insert_signature":
		if strings.TrimSpace(op.SignatureID) == "" {
			return fmt.Errorf("insert_signature requires signature_id")
		}
	case "remove_signature":
		// No required fields.
	case "set_calendar":
		if strings.TrimSpace(op.EventSummary) == "" {
			return fmt.Errorf("set_calendar requires event_summary")
		}
		if strings.TrimSpace(op.EventStart) == "" || strings.TrimSpace(op.EventEnd) == "" {
			return fmt.Errorf("set_calendar requires event_start and event_end")
		}
		start, err := parseISO8601(op.EventStart)
		if err != nil {
			return fmt.Errorf("set_calendar: event_start must be a valid ISO 8601 timestamp")
		}
		end, err := parseISO8601(op.EventEnd)
		if err != nil {
			return fmt.Errorf("set_calendar: event_end must be a valid ISO 8601 timestamp")
		}
		if !end.After(start) {
			return fmt.Errorf("set_calendar: event_end must be after event_start")
		}
	case "remove_calendar":
		// No required fields.
	default:
		return fmt.Errorf("unsupported op %q", op.Op)
	}
	return nil
}

func isRecipientField(field string) bool {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "to", "cc", "bcc":
		return true
	default:
		return false
	}
}

func isBodyKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "text/plain", "text/html":
		return true
	default:
		return false
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func formatAddressList(addrs []Address) string {
	parts := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if strings.TrimSpace(addr.Address) == "" {
			continue
		}
		parts = append(parts, addr.String())
	}
	return strings.Join(parts, ", ")
}

func decodeHeaderValue(value string) string {
	dec := new(mime.WordDecoder)
	decoded, err := dec.DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func (p Patch) Summary() map[string]interface{} {
	out := map[string]interface{}{
		"ops":      p.Ops,
		"warnings": []string{"该编辑链路不具备乐观锁保护；若草稿被并发修改，后写入者会覆盖前者"},
	}
	if p.Options != (PatchOptions{}) {
		out["options"] = p.Options
	}
	return out
}

func MustJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("MustJSON: %v", err))
	}
	return string(data)
}

// parseISO8601 tries common ISO 8601 timestamp layouts, accepting both
// with-seconds (RFC 3339) and without-seconds variants.
func parseISO8601(s string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q as ISO 8601", s)
}
