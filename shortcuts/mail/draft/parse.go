// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // intermediate draft EML parser errors; mail command layer wraps into typed ValidationError.
package draft

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
)

func Parse(raw DraftRaw) (*DraftSnapshot, error) {
	decoded, err := decodeRawEML(raw.RawEML)
	if err != nil {
		return nil, err
	}
	headers, body, err := parseHeaderBlock(decoded)
	if err != nil {
		return nil, err
	}
	root, err := parseRootPart(headers, body)
	if err != nil {
		return nil, err
	}
	snapshot := &DraftSnapshot{
		DraftID: raw.DraftID,
		Headers: headers,
		Body:    root,
	}
	if err := refreshSnapshot(snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// maxRawEMLSize is the maximum accepted raw (base64-encoded) EML string length.
// Base64 encodes 3 bytes into 4 chars, so 35 MB covers a 25 MB decoded EML with margin.
const maxRawEMLSize = 35 * 1024 * 1024

func decodeRawEML(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("draft raw EML is empty")
	}
	if len(raw) > maxRawEMLSize {
		return nil, fmt.Errorf("draft raw EML is too large (%d bytes, max %d)", len(raw), maxRawEMLSize)
	}
	decoders := []func(string) ([]byte, error){
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
	}
	for _, decode := range decoders {
		decoded, err := decode(raw)
		if err == nil {
			return normalizeLineEndings(decoded), nil
		}
	}
	return nil, fmt.Errorf("draft raw EML is not valid base64url")
}

func normalizeLineEndings(in []byte) []byte {
	in = bytes.ReplaceAll(in, []byte("\r\n"), []byte("\n"))
	in = bytes.ReplaceAll(in, []byte("\r"), []byte("\n"))
	return in
}

func parseHeaderBlock(raw []byte) ([]Header, []byte, error) {
	raw = normalizeLineEndings(raw)
	sep := bytes.Index(raw, []byte("\n\n"))
	if sep < 0 {
		return nil, nil, fmt.Errorf("invalid EML: missing header/body separator")
	}
	headerLines := strings.Split(string(raw[:sep]), "\n")
	headers := make([]Header, 0, len(headerLines))
	for _, line := range headerLines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && len(headers) > 0 {
			headers[len(headers)-1].Value += " " + strings.TrimSpace(line)
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			// Skip lines without a colon rather than failing. Some email
			// systems insert comment or separator lines in the header area.
			continue
		}
		headers = append(headers, Header{
			Name:  strings.TrimSpace(name),
			Value: strings.TrimSpace(value),
		})
	}
	return headers, raw[sep+2:], nil
}

func parseRootPart(messageHeaders []Header, body []byte) (*Part, error) {
	partHeaders := extractBodyHeaders(messageHeaders)
	part := &Part{
		PartID:  "1",
		Headers: append([]Header{}, partHeaders...),
	}
	if len(partHeaders) == 0 {
		part.MediaType = "text/plain"
		part.MediaParams = map[string]string{"charset": "UTF-8"}
		part.TransferEncoding = "7bit"
		part.Body = body
		part.RawEntity = append([]byte{}, body...)
		return part, nil
	}
	rawEntity := buildRawEntity(filterRawEntityHeaders(partHeaders), body)
	return parsePart(partHeaders, body, "1", rawEntity, 0)
}

const maxMIMEDepth = 50

func parsePart(headers []Header, body []byte, partID string, rawEntity []byte, depth int) (*Part, error) {
	if depth > maxMIMEDepth {
		return nil, fmt.Errorf("MIME nesting too deep (max %d levels)", maxMIMEDepth)
	}
	part := &Part{
		PartID:                partID,
		Headers:               append([]Header{}, headers...),
		MediaType:             "text/plain",
		MediaParams:           map[string]string{},
		ContentDispositionArg: map[string]string{},
		RawEntity:             append([]byte{}, rawEntity...),
	}
	if ct := headerValue(headers, "Content-Type"); ct != "" {
		mediaType, params, err := mime.ParseMediaType(ct)
		if err != nil {
			// Fallback: treat as opaque binary so the part is still accessible
			// and can round-trip through RawEntity. The original Content-Type
			// header is preserved for serialization.
			part.MediaType = "application/octet-stream"
			part.EncodingProblem = true
		} else {
			part.MediaType = strings.ToLower(mediaType)
			part.MediaParams = lowerCaseKeys(params)
		}
	} else {
		part.MediaParams["charset"] = "UTF-8"
	}
	if disp := headerValue(headers, "Content-Disposition"); disp != "" {
		dispType, params, err := mime.ParseMediaType(disp)
		if err == nil {
			part.ContentDisposition = strings.ToLower(dispType)
			part.ContentDispositionArg = lowerCaseKeys(params)
		}
		// On parse error, silently ignore the disposition. The original
		// header is preserved in part.Headers for serialization.
	}
	part.ContentID = strings.Trim(strings.TrimSpace(headerValue(headers, "Content-ID")), "<>")
	part.TransferEncoding = strings.ToLower(strings.TrimSpace(headerValue(headers, "Content-Transfer-Encoding")))

	if strings.HasPrefix(part.MediaType, "multipart/") {
		boundary := part.MediaParams["boundary"]
		if boundary == "" {
			return nil, fmt.Errorf("multipart part %s missing boundary", partID)
		}
		children, preamble, epilogue, err := parseMultipartChildren(body, boundary, partID, depth)
		if err != nil {
			return nil, err
		}
		if len(children) == 0 {
			// Boundary declared but never found in the body. Reclassify as
			// text rather than returning an empty multipart with no children
			// (following mail-parser's approach per Postel's law).
			part.MediaType = "text/plain"
			part.MediaParams = map[string]string{"charset": "UTF-8"}
			part.Body = body
			part.EncodingProblem = true
			return part, nil
		}
		part.Children = children
		part.Preamble = preamble
		part.Epilogue = epilogue
		return part, nil
	}

	decoded, encodingProblem := decodePartBody(body, part.TransferEncoding, part.MediaType, part.MediaParams)
	part.Body = decoded
	if encodingProblem {
		part.EncodingProblem = true
	}
	return part, nil
}

func parseMultipartChildren(body []byte, boundary, parentPartID string, depth int) ([]*Part, []byte, []byte, error) {
	lines := bytes.SplitAfter(body, []byte("\n"))
	startLine := "--" + boundary
	endLine := "--" + boundary + "--"

	var (
		children []*Part
		preamble bytes.Buffer
		epilogue bytes.Buffer
		buf      bytes.Buffer
		inPart   bool
		afterEnd bool
		index    int
	)

	flush := func() error {
		// Copy buf content before Reset to avoid memory aliasing: buf.Bytes()
		// returns a sub-slice of buf's internal array which gets overwritten
		// when the next MIME part is written to buf after Reset.
		partBytes := append([]byte{}, bytes.TrimRight(buf.Bytes(), " \t\r\n")...)
		buf.Reset()
		if len(partBytes) == 0 {
			return nil
		}
		// Ensure the header/body separator (\n\n) is present so parseHeaderBlock
		// can split headers from body.
		//
		// A part whose first byte is \n has no headers (the \n is the blank-line
		// separator). Prepend an extra \n so parseHeaderBlock finds \n\n at
		// position 0 and returns empty headers.
		//
		// Otherwise the part has headers but TrimRight may have stripped the
		// trailing \n\n when the body was empty. Re-append it.
		if len(partBytes) > 0 && partBytes[0] == '\n' {
			partBytes = append([]byte{'\n'}, partBytes...)
		} else if !bytes.Contains(partBytes, []byte("\n\n")) {
			partBytes = append(partBytes, '\n', '\n')
		}
		index++
		partID := fmt.Sprintf("%s.%d", parentPartID, index)
		headers, body, err := parseHeaderBlock(partBytes)
		if err != nil {
			return err
		}
		child, err := parsePart(headers, body, partID, partBytes, depth+1)
		if err != nil {
			return err
		}
		children = append(children, child)
		return nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(string(line))
		if afterEnd {
			epilogue.Write(line)
			continue
		}
		switch trimmed {
		case startLine:
			if inPart {
				if err := flush(); err != nil {
					return nil, nil, nil, err
				}
			} else {
				inPart = true
				continue
			}
			inPart = true
		case endLine:
			if inPart {
				if err := flush(); err != nil {
					return nil, nil, nil, err
				}
			}
			afterEnd = true
		default:
			if inPart {
				buf.Write(line)
			} else {
				preamble.Write(line)
			}
		}
	}
	if inPart {
		if err := flush(); err != nil {
			return nil, nil, nil, err
		}
	}
	return children, preamble.Bytes(), epilogue.Bytes(), nil
}

// decodePartBody decodes the transfer-encoded body. The second return value
// indicates whether a decoding problem occurred (the returned bytes are a
// best-effort fallback in that case).
func decodePartBody(body []byte, cte, mediaType string, mediaParams map[string]string) ([]byte, bool) {
	var decoded []byte
	var problem bool
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "", "7bit", "8bit", "binary":
		decoded = body
	case "base64":
		trimmed := strings.Join(strings.Fields(string(body)), "")
		var err error
		decoded, err = base64.StdEncoding.DecodeString(trimmed)
		if err != nil {
			// Try without padding — some implementations omit it.
			decoded, err = base64.RawStdEncoding.DecodeString(trimmed)
		}
		if err != nil {
			// Give up decoding; keep raw bytes so the part is still
			// accessible and can round-trip through RawEntity.
			decoded = body
			problem = true
		}
	case "quoted-printable":
		var err error
		decoded, err = io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body)))
		if err != nil {
			// Keep raw bytes on QP decode failure (same as base64 fallback).
			decoded = body
			problem = true
		}
	default:
		decoded = body
	}
	if !isTextualMediaType(mediaType) {
		return decoded, problem
	}
	text, err := decodeTextCharset(decoded, mediaParams["charset"])
	if err != nil {
		// Unsupported or malformed charsets should not break draft parsing.
		// Keep the decoded transfer bytes so untouched subtrees can still round-trip raw.
		return decoded, true
	}
	return text, problem
}

func extractBodyHeaders(headers []Header) []Header {
	out := make([]Header, 0, len(headers))
	for _, header := range headers {
		if isBodyHeader(header.Name) {
			out = append(out, header)
		}
	}
	return out
}

func buildRawEntity(headers []Header, body []byte) []byte {
	if len(headers) == 0 {
		return append([]byte{}, body...)
	}
	var buf bytes.Buffer
	for _, header := range headers {
		buf.WriteString(header.Name)
		buf.WriteString(": ")
		buf.WriteString(header.Value)
		buf.WriteByte('\n')
	}
	buf.WriteByte('\n')
	buf.Write(body)
	return buf.Bytes()
}

func filterRawEntityHeaders(headers []Header) []Header {
	out := make([]Header, 0, len(headers))
	for _, header := range headers {
		if strings.EqualFold(header.Name, "MIME-Version") {
			continue
		}
		out = append(out, header)
	}
	return out
}

func refreshSnapshot(snapshot *DraftSnapshot) error {
	snapshot.Subject = decodeHeaderValue(headerValue(snapshot.Headers, "Subject"))
	snapshot.MessageID = strings.TrimSpace(headerValue(snapshot.Headers, "Message-ID"))
	snapshot.InReplyTo = strings.TrimSpace(headerValue(snapshot.Headers, "In-Reply-To"))
	snapshot.References = strings.TrimSpace(headerValue(snapshot.Headers, "References"))

	// Address headers are parsed leniently: malformed addresses (non-standard
	// display names, semicolons, broken encoding) are silently ignored so that
	// the draft can still be opened. The raw header values are preserved in
	// snapshot.Headers for round-trip serialization.
	snapshot.From, _ = parseAddressHeader(headerValue(snapshot.Headers, "From"))
	snapshot.To, _ = parseAddressHeader(headerValue(snapshot.Headers, "To"))
	snapshot.Cc, _ = parseAddressHeader(headerValue(snapshot.Headers, "Cc"))
	snapshot.Bcc, _ = parseAddressHeader(headerValue(snapshot.Headers, "Bcc"))
	snapshot.ReplyTo, _ = parseAddressHeader(headerValue(snapshot.Headers, "Reply-To"))

	reindexParts(snapshot.Body, "1")
	textPart := findPrimaryBodyPart(snapshot.Body, "text/plain")
	htmlPart := findPrimaryBodyPart(snapshot.Body, "text/html")
	if textPart != nil {
		snapshot.PrimaryTextPartID = textPart.PartID
	} else {
		snapshot.PrimaryTextPartID = ""
	}
	if htmlPart != nil {
		snapshot.PrimaryHTMLPartID = htmlPart.PartID
	} else {
		snapshot.PrimaryHTMLPartID = ""
	}
	// Inline CID consistency is NOT validated here — broken CID references
	// should not prevent opening the draft editor. Project() already reports
	// missing CIDs as warnings in DraftProjection.Warnings.
	return nil
}

func parseAddressHeader(value string) ([]Address, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parser := &mail.AddressParser{WordDecoder: new(mime.WordDecoder)}
	addrs, err := parser.ParseList(value)
	if err != nil {
		return nil, err
	}
	out := make([]Address, 0, len(addrs))
	for _, addr := range addrs {
		out = append(out, Address{
			Name:    addr.Name,
			Address: strings.TrimSpace(addr.Address),
		})
	}
	return out, nil
}

func lowerCaseKeys(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[strings.ToLower(k)] = v
	}
	return out
}

func headerValue(headers []Header, name string) string {
	for _, header := range headers {
		if strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}
	return ""
}

func isBodyHeader(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(name, "content-") || name == "mime-version"
}

func reindexParts(part *Part, partID string) {
	if part == nil {
		return
	}
	part.PartID = partID
	for i, child := range part.Children {
		reindexParts(child, fmt.Sprintf("%s.%d", partID, i+1))
	}
}

func findPrimaryBodyPart(root *Part, mediaType string) *Part {
	var best *Part
	bestScore := -1

	var walk func(part *Part, ancestors []string)
	walk = func(part *Part, ancestors []string) {
		if part == nil {
			return
		}
		if !part.IsMultipart() {
			score, ok := bodyCandidateScore(part, ancestors, mediaType)
			if ok && score > bestScore {
				best = part
				bestScore = score
			}
			return
		}
		nextAncestors := append(append([]string{}, ancestors...), part.MediaType)
		for _, child := range part.Children {
			walk(child, nextAncestors)
		}
	}
	walk(root, nil)
	return best
}

func bodyCandidateScore(part *Part, ancestors []string, mediaType string) (int, bool) {
	if part == nil || !strings.EqualFold(part.MediaType, mediaType) {
		return 0, false
	}
	switch strings.ToLower(part.ContentDisposition) {
	case "attachment", "inline":
		return 0, false
	}
	score := 1
	for _, ancestor := range ancestors {
		switch ancestor {
		case "multipart/signed", "multipart/encrypted":
			return 0, false
		case "multipart/alternative":
			score += 10
		case "multipart/related":
			if mediaType == "text/html" {
				score += 5
			}
		}
	}
	return score, true
}
