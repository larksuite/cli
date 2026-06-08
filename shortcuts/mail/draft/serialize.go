// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // intermediate draft serializer errors; mail command layer wraps into typed ValidationError.
package draft

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math/rand"
	"mime"
	"mime/quotedprintable"
	"strings"
)

func Serialize(snapshot *DraftSnapshot) (string, error) {
	if snapshot == nil || snapshot.Body == nil {
		return "", fmt.Errorf("draft snapshot is empty")
	}
	var buf bytes.Buffer
	mimeVersionValue := "1.0"
	wroteMimeVersion := false
	for _, header := range snapshot.Headers {
		if strings.EqualFold(header.Name, "MIME-Version") {
			mimeVersionValue = header.Value
			writeHeader(&buf, header.Name, header.Value)
			wroteMimeVersion = true
			continue
		}
		if isBodyHeader(header.Name) {
			continue
		}
		writeHeader(&buf, header.Name, header.Value)
	}
	if !wroteMimeVersion {
		writeHeader(&buf, "MIME-Version", mimeVersionValue)
	}
	if err := writeTopLevelBody(&buf, snapshot.Body); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf.Bytes()), nil
}

func writeTopLevelBody(buf *bytes.Buffer, root *Part) error {
	if canReuseRawEntity(root) {
		buf.Write(root.RawEntity)
		if len(root.RawEntity) == 0 || root.RawEntity[len(root.RawEntity)-1] != '\n' {
			buf.WriteByte('\n')
		}
		return nil
	}
	if root.IsMultipart() {
		for _, header := range orderedPartHeaders(root, false) {
			writeHeader(buf, header.Name, header.Value)
		}
		buf.WriteByte('\n')
		return writeMultipartBody(buf, root)
	}
	for _, header := range orderedPartHeaders(root, true) {
		writeHeader(buf, header.Name, header.Value)
	}
	buf.WriteByte('\n')
	return writeLeafBody(buf, root)
}

func writeMultipartBody(buf *bytes.Buffer, part *Part) error {
	boundary := part.MediaParams["boundary"]
	if boundary == "" {
		boundary = newBoundary()
		part.MediaParams["boundary"] = boundary
	}
	if len(part.Preamble) > 0 {
		buf.Write(part.Preamble)
		if part.Preamble[len(part.Preamble)-1] != '\n' {
			buf.WriteByte('\n')
		}
	}
	for _, child := range part.Children {
		if child == nil {
			continue
		}
		fmt.Fprintf(buf, "--%s\n", boundary)
		if canReuseRawEntity(child) {
			buf.Write(child.RawEntity)
			if n := len(child.RawEntity); n == 0 || child.RawEntity[n-1] != '\n' {
				buf.WriteByte('\n')
			}
			continue
		}
		if child.IsMultipart() {
			for _, header := range orderedPartHeaders(child, false) {
				writeHeader(buf, header.Name, header.Value)
			}
			buf.WriteByte('\n')
			if err := writeMultipartBody(buf, child); err != nil {
				return err
			}
			continue
		}
		for _, header := range orderedPartHeaders(child, true) {
			writeHeader(buf, header.Name, header.Value)
		}
		buf.WriteByte('\n')
		if err := writeLeafBody(buf, child); err != nil {
			return err
		}
	}
	fmt.Fprintf(buf, "--%s--\n", boundary)
	if len(part.Epilogue) > 0 {
		buf.Write(part.Epilogue)
		if part.Epilogue[len(part.Epilogue)-1] != '\n' {
			buf.WriteByte('\n')
		}
	}
	return nil
}

func orderedPartHeaders(part *Part, includeCTE bool) []Header {
	contentTypeValue := existingHeaderValue(part.Headers, "Content-Type")
	if contentTypeValue == "" {
		contentTypeValue = mime.FormatMediaType(part.MediaType, cloneStringMap(part.MediaParams))
	}

	headers := make([]Header, 0, len(part.Headers)+4)
	replacements := map[string]Header{
		"content-type": {
			Name:  "Content-Type",
			Value: contentTypeValue,
		},
	}
	if includeCTE {
		if cte := chooseTransferEncoding(part); cte != "" {
			value := cte
			if existing := existingHeaderValue(part.Headers, "Content-Transfer-Encoding"); strings.EqualFold(existing, cte) {
				value = existing
			}
			replacements["content-transfer-encoding"] = Header{
				Name:  "Content-Transfer-Encoding",
				Value: value,
			}
		}
	}
	if part.ContentDisposition != "" {
		value := existingHeaderValue(part.Headers, "Content-Disposition")
		if value == "" {
			value = mime.FormatMediaType(part.ContentDisposition, cloneStringMap(part.ContentDispositionArg))
		}
		replacements["content-disposition"] = Header{
			Name:  "Content-Disposition",
			Value: value,
		}
	}
	if part.ContentID != "" {
		value := existingHeaderValue(part.Headers, "Content-ID")
		if value == "" {
			value = "<" + part.ContentID + ">"
		}
		replacements["content-id"] = Header{Name: "Content-ID", Value: value}
	}

	written := make(map[string]bool, len(replacements))
	for _, header := range part.Headers {
		name := strings.ToLower(header.Name)
		switch name {
		case "mime-version":
			continue
		case "content-type", "content-transfer-encoding", "content-disposition", "content-id":
			if replacement, ok := replacements[name]; ok {
				replacement.Name = header.Name
				headers = append(headers, replacement)
				written[name] = true
			}
		default:
			headers = append(headers, header)
		}
	}
	for _, key := range []string{"content-type", "content-transfer-encoding", "content-disposition", "content-id"} {
		if written[key] {
			continue
		}
		if replacement, ok := replacements[key]; ok {
			headers = append(headers, replacement)
		}
	}
	return headers
}

func chooseTransferEncoding(part *Part) string {
	if part.IsMultipart() {
		return ""
	}
	switch {
	case part.ContentDisposition == "attachment":
		return "base64"
	case strings.HasPrefix(part.MediaType, "text/"):
		switch strings.ToLower(strings.TrimSpace(part.TransferEncoding)) {
		case "quoted-printable":
			return "quoted-printable"
		case "base64":
			if hasNonASCII(part.Body) {
				return "base64"
			}
		}
		if hasNonASCII(part.Body) {
			return "quoted-printable"
		}
		return "7bit"
	default:
		return "base64"
	}
}

func writeLeafBody(buf *bytes.Buffer, part *Part) error {
	body, err := encodedLeafBody(part)
	if err != nil {
		return err
	}
	cte := chooseTransferEncoding(part)
	switch cte {
	case "base64":
		writeFoldedBody(buf, base64.StdEncoding.EncodeToString(body), 76)
	case "quoted-printable":
		writer := quotedprintable.NewWriter(buf)
		if _, err := writer.Write(body); err != nil {
			_ = writer.Close()
			return err
		}
		if err := writer.Close(); err != nil {
			return err
		}
		if buf.Len() == 0 || buf.Bytes()[buf.Len()-1] != '\n' {
			buf.WriteByte('\n')
		}
	default:
		if len(body) > 0 {
			buf.Write(body)
			if body[len(body)-1] != '\n' {
				buf.WriteByte('\n')
			}
		} else {
			buf.WriteByte('\n')
		}
	}
	return nil
}

func writeFoldedBody(buf *bytes.Buffer, encoded string, width int) {
	if width <= 0 {
		width = 76
	}
	for len(encoded) > width {
		buf.WriteString(encoded[:width])
		buf.WriteByte('\n')
		encoded = encoded[width:]
	}
	if encoded != "" {
		buf.WriteString(encoded)
		buf.WriteByte('\n')
	}
}

func writeHeader(buf *bytes.Buffer, name, value string) {
	// Strip CR and LF as a last-resort defense against header injection.
	// Callers (applyOp, Validate) already reject CR/LF explicitly; this
	// sanitisation covers any path that bypasses those checks.
	name = strings.NewReplacer("\r", "", "\n", "").Replace(name)
	value = strings.NewReplacer("\r", "", "\n", "").Replace(value)
	buf.WriteString(name)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteByte('\n')
}

func existingHeaderValue(headers []Header, name string) string {
	for _, header := range headers {
		if strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}
	return ""
}

func canReuseRawEntity(part *Part) bool {
	if part == nil || len(part.RawEntity) == 0 {
		return false
	}
	return !partHasDirty(part)
}

func partHasDirty(part *Part) bool {
	if part == nil {
		return false
	}
	if part.Dirty {
		return true
	}
	for _, child := range part.Children {
		if partHasDirty(child) {
			return true
		}
	}
	return false
}

func hasNonASCII(body []byte) bool {
	for _, b := range body {
		if b > 127 {
			return true
		}
	}
	return false
}

func encodedLeafBody(part *Part) ([]byte, error) {
	if part == nil {
		return nil, nil
	}
	if !isTextualMediaType(part.MediaType) {
		return part.Body, nil
	}
	charsetLabel := normalizeCharsetLabel(part.MediaParams["charset"])
	if charsetLabel == "" {
		charsetLabel = "UTF-8"
		part.MediaParams["charset"] = charsetLabel
	}
	encoded, err := encodeTextCharset(part.Body, charsetLabel)
	if err == nil {
		return encoded, nil
	}
	part.MediaParams["charset"] = "UTF-8"
	syncStructuredPartHeaders(part)
	return part.Body, nil
}

func newBoundary() string {
	return fmt.Sprintf("lark-draft-%d-%d", rand.Int63(), rand.Int63())
}
