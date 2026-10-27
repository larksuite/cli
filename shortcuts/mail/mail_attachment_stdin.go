// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
)

// Narrow byte input, deliberately not a filesystem path or a general upload API.
const maxStdinAttachmentBytes int64 = 10 * 1024 * 1024
const maxStdinManifestBytes = 256 * 1024

var attachStdinNameFlag = common.Flag{Name: "attach-stdin-name", Desc: "Read one authorized PDF attachment from stdin, at most 10 MiB; explicit basename ending in .pdf required. No path is opened. Cannot share stdin with other inputs. Dry-run validates metadata only."}
var attachStdinTypeFlag = common.Flag{Name: "attach-stdin-type", Desc: "Required with --attach-stdin-name: application/pdf. PDF magic is checked; this is not a malware scan. Regular attachments use the backend-compatible octet-stream MIME type."}
var attachStdinManifestFlag = common.Flag{Name: "attach-stdin-manifest", Desc: "JSON argv array of {filename,size} for 1-250 authorized PDFs; stdin is their original bytes concatenated in order, exactly size bytes each. Combined maximum 10 MiB. Basenames ending in .pdf and PDF magic required; no paths opened. Exclusive with single-file flags and other stdin inputs. Dry-run validates metadata only; no malware scan."}

type stdinAttachment struct {
	Name string
	Data []byte
}
type stdinAttachments []stdinAttachment
type stdinAttachmentMetadata struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// Reject ambiguous keys rather than accepting encoding/json's duplicate/case aliases.
func (m *stdinAttachmentMetadata) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fmt.Errorf("expected metadata object")
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return fmt.Errorf("duplicate metadata key")
		}
		seen[key] = true
		switch key {
		case "filename":
			err = decoder.Decode(&m.Filename)
		case "size":
			err = decoder.Decode(&m.Size)
		default:
			return fmt.Errorf("unknown metadata key")
		}
		if err != nil {
			return err
		}
	}
	if _, err = decoder.Token(); err != nil {
		return err
	}
	if !seen["filename"] || !seen["size"] {
		return fmt.Errorf("missing metadata key")
	}
	return nil
}

func validateStdinAttachmentFlags(r *common.RuntimeContext) error {
	_, err := stdinAttachmentMetadataFromFlags(r)
	return err
}

func stdinAttachmentMetadataFromFlags(r *common.RuntimeContext) ([]stdinAttachmentMetadata, error) {
	name, kind, manifest := r.Str("attach-stdin-name"), r.Str("attach-stdin-type"), r.Str("attach-stdin-manifest")
	if manifest != "" {
		if name != "" || kind != "" {
			return nil, mailValidationParamError("--attach-stdin-manifest", "cannot combine manifest and single-file stdin flags")
		}
		if !utf8.ValidString(manifest) {
			return nil, mailValidationParamError("--attach-stdin-manifest", "manifest must be valid UTF-8")
		}
		if len(manifest) > maxStdinManifestBytes {
			return nil, mailValidationParamError("--attach-stdin-manifest", "manifest exceeds 256 KiB")
		}
		var entries []stdinAttachmentMetadata
		decoder := json.NewDecoder(strings.NewReader(manifest))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&entries); err != nil {
			return nil, mailValidationParamError("--attach-stdin-manifest", "expected JSON array of {filename,size}")
		}
		if err := decoder.Decode(new(interface{})); err != io.EOF {
			return nil, mailValidationParamError("--attach-stdin-manifest", "unexpected trailing JSON")
		}
		if len(entries) == 0 || len(entries) > MaxAttachmentCount {
			return nil, mailValidationParamError("--attach-stdin-manifest", "manifest requires 1-250 PDF attachments")
		}
		var total int64
		for _, entry := range entries {
			if !validStdinPDFName(entry.Filename) {
				return nil, mailValidationParamError("--attach-stdin-manifest", "each filename must be a .pdf basename without paths or control characters")
			}
			if entry.Size < 5 || entry.Size > maxStdinAttachmentBytes-total {
				return nil, mailValidationParamError("--attach-stdin-manifest", "each PDF must be at least 5 bytes and combined size must not exceed 10 MiB")
			}
			total += entry.Size
		}
		return entries, nil
	}
	if name == "" && kind == "" {
		return nil, nil
	}
	if !validStdinPDFName(name) {
		return nil, mailValidationParamError("--attach-stdin-name", "stdin attachment requires a .pdf basename without paths or control characters")
	}
	if kind != "application/pdf" {
		return nil, mailValidationParamError("--attach-stdin-type", "stdin PDF attachment requires explicit --attach-stdin-type application/pdf")
	}
	return []stdinAttachmentMetadata{{Filename: name}}, nil
}

func validStdinPDFName(name string) bool {
	return name != "" && strings.TrimSpace(name) == name && name != "." && name != ".." && !strings.ContainsAny(name, "/\\:") && utf8.ValidString(name) && strings.IndexFunc(name, unicode.IsControl) < 0 && strings.HasSuffix(strings.ToLower(name), ".pdf")
}

func readStdinAttachment(r *common.RuntimeContext) (stdinAttachments, error) {
	entries, err := stdinAttachmentMetadataFromFlags(r)
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	flag := "--attach-stdin-name"
	if r.Str("attach-stdin-manifest") != "" {
		flag = "--attach-stdin-manifest"
	}
	data, err := r.ReadStdinBytesOnce(flag, maxStdinAttachmentBytes)
	if err != nil {
		return nil, err
	}
	if flag == "--attach-stdin-name" {
		entries[0].Size = int64(len(data))
	}
	var total int64
	for _, entry := range entries {
		total += entry.Size
	}
	if int64(len(data)) != total {
		return nil, mailValidationParamError(flag, "stdin length does not match declared PDF sizes (truncated or trailing bytes)")
	}
	attachments := make(stdinAttachments, 0, len(entries))
	offset := int64(0)
	for _, entry := range entries {
		payload := data[offset : offset+entry.Size]
		if !bytes.HasPrefix(payload, []byte("%PDF-")) {
			return nil, mailValidationParamError(flag, "stdin attachment is empty or lacks the PDF header")
		}
		attachments = append(attachments, stdinAttachment{Name: entry.Filename, Data: payload})
		offset += entry.Size
	}
	return attachments, nil
}
func (a stdinAttachments) addTo(b emlbuilder.Builder) emlbuilder.Builder {
	for _, file := range a {
		b = b.AddAttachment(file.Data, "application/octet-stream", file.Name)
	}
	return b
}
func (a stdinAttachments) count() int { return len(a) }
func (a stdinAttachments) emlBytes() int64 {
	var total int64
	for _, file := range a {
		total += int64(len(file.Data))*137/100 + 1024
	}
	return total
}
