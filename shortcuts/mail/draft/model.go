// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/mail"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
)

type DraftRaw struct {
	DraftID string
	RawEML  string
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

type DraftProjection struct {
	Subject            string        `json:"subject"`
	To                 []Address     `json:"to,omitempty"`
	Cc                 []Address     `json:"cc,omitempty"`
	Bcc                []Address     `json:"bcc,omitempty"`
	ReplyTo            []Address     `json:"reply_to,omitempty"`
	InReplyTo          string        `json:"in_reply_to,omitempty"`
	References         string        `json:"references,omitempty"`
	BodyText           string        `json:"body_text,omitempty"`
	BodyHTMLSummary    string        `json:"body_html_summary,omitempty"`
	HasQuotedContent   bool          `json:"has_quoted_content,omitempty"`
	AttachmentsSummary []PartSummary `json:"attachments_summary,omitempty"`
	InlineSummary      []PartSummary `json:"inline_summary,omitempty"`
	Warnings           []string      `json:"warnings,omitempty"`
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
}

func (t AttachmentTarget) hasKey() bool {
	return strings.TrimSpace(t.PartID) != "" || strings.TrimSpace(t.CID) != ""
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
		if !op.Target.hasKey() {
			return fmt.Errorf("remove_attachment requires target with at least one of part_id or cid")
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
