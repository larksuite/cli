// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
)

// largeAttHeaderEntry is a union of the CLI and server JSON formats for
// entries in the large attachment header.
type largeAttHeaderEntry struct {
	ID       string `json:"id,omitempty"`
	FileKey  string `json:"file_key,omitempty"`
	FileName string `json:"file_name,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
}

func (e largeAttHeaderEntry) token() string {
	if e.ID != "" {
		return e.ID
	}
	return e.FileKey
}

// IsLargeAttachmentHeader returns true if the header name matches either
// the CLI-written or server-returned large attachment header.
func IsLargeAttachmentHeader(name string) bool {
	return strings.EqualFold(name, LargeAttachmentIDsHeader) ||
		strings.EqualFold(name, ServerLargeAttachmentHeader)
}

// decodeLargeAttachmentHeader decodes the base64 value and returns entries.
func decodeLargeAttachmentHeader(value string) ([]largeAttHeaderEntry, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	var items []largeAttHeaderEntry
	if err := json.Unmarshal(decoded, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// parseLargeAttachmentTokens returns the ordered list of large attachment
// tokens from either X-Lms-Large-Attachment-Ids (CLI format) or
// X-Lark-Large-Attachment (server format). Returns nil when neither
// header is present or the value is malformed.
func parseLargeAttachmentTokens(headers []Header) []string {
	for _, h := range headers {
		if !IsLargeAttachmentHeader(h.Name) {
			continue
		}
		items, err := decodeLargeAttachmentHeader(h.Value)
		if err != nil {
			return nil
		}
		out := make([]string, 0, len(items))
		for _, it := range items {
			if tok := it.token(); tok != "" {
				out = append(out, tok)
			}
		}
		return out
	}
	return nil
}

// ParseLargeAttachmentSummariesFromHeader extracts full metadata from the
// large attachment header. Returns non-nil only when the server-format
// header (X-Lark-Large-Attachment) is found, since it carries file_name
// and file_size that the CLI-format header lacks.
func ParseLargeAttachmentSummariesFromHeader(headers []Header) []LargeAttachmentSummary {
	for _, h := range headers {
		if !strings.EqualFold(h.Name, ServerLargeAttachmentHeader) {
			continue
		}
		items, err := decodeLargeAttachmentHeader(h.Value)
		if err != nil {
			return nil
		}
		out := make([]LargeAttachmentSummary, 0, len(items))
		for _, it := range items {
			tok := it.token()
			if tok == "" {
				continue
			}
			out = append(out, LargeAttachmentSummary{
				Token:     tok,
				FileName:  it.FileName,
				SizeBytes: it.FileSize,
			})
		}
		return out
	}
	return nil
}

// ParseLargeAttachmentItemsFromHTML walks the HTML body looking for large
// attachment card items (<div id="large-file-item">) and returns a map
// from token (data-mail-token attribute value) to filename + size.
//
// The size is parsed best-effort from the displayed string (e.g. "25.0 MB");
// it carries the precision of the formatted value and is not byte-exact.
func ParseLargeAttachmentItemsFromHTML(htmlBody string) map[string]LargeAttachmentSummary {
	out := map[string]LargeAttachmentSummary{}
	if htmlBody == "" {
		return out
	}
	doc, err := xhtml.Parse(strings.NewReader(htmlBody))
	if err != nil {
		return out
	}
	var walk func(n *xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n == nil {
			return
		}
		if n.Type == xhtml.ElementNode && n.Data == "div" && attr(n, "id") == LargeFileItemID {
			if token, meta, ok := extractItemMeta(n); ok {
				out[token] = meta
			}
			// Do not descend further: the <a> and texts have been collected.
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

// extractItemMeta collects the token, filename, and size from a large
// attachment item node. Returns ok=false when the token is missing.
//
// Expected structure (see largeAttItemTpl in mail/large_attachment.go):
//
//	<div id="large-file-item">
//	  <div><img ... /></div>               // icon
//	  <div>
//	    <div>FILENAME</div>
//	    <div><span>SIZE_DISPLAY</span></div>
//	  </div>
//	  <a data-mail-token="TOKEN" ...>DOWNLOAD_LABEL</a>
//	</div>
//
// The token comes from the <a data-mail-token=...>. The first non-anchor
// text is the filename; the next text is the size display.
func extractItemMeta(item *xhtml.Node) (token string, meta LargeAttachmentSummary, ok bool) {
	var texts []string
	var insideAnchor bool

	var walk func(n *xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n == nil {
			return
		}
		if n.Type == xhtml.ElementNode && n.Data == "a" {
			if t := attr(n, LargeAttachmentTokenAttr); t != "" && token == "" {
				token = t
			}
			// Skip collecting the anchor's label (e.g. "Download" / "下载").
			prev := insideAnchor
			insideAnchor = true
			defer func() { insideAnchor = prev }()
		}
		if n.Type == xhtml.TextNode && !insideAnchor {
			if s := strings.TrimSpace(n.Data); s != "" {
				texts = append(texts, s)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(item)

	if token == "" {
		return "", LargeAttachmentSummary{}, false
	}
	if len(texts) > 0 {
		meta.FileName = texts[0]
	}
	if len(texts) > 1 {
		meta.SizeBytes = parseSizeDisplay(texts[1])
	}
	return token, meta, true
}

func attr(n *xhtml.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// sizeDisplayRe matches sizes like "25.0 MB", "1 GB", "500 KB", "42 B".
// The unit is case-insensitive and may be B / KB / MB / GB / TB.
var sizeDisplayRe = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*(B|KB|MB|GB|TB)\s*$`)

// parseSizeDisplay converts a formatted size display string back into
// an approximate byte count. Precision is limited by the display rounding
// (e.g. "25.0 MB" round-trips to 26214400 bytes).
// Returns 0 when the input cannot be parsed.
func parseSizeDisplay(s string) int64 {
	m := sizeDisplayRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	value, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	unit := strings.ToUpper(m[2])
	var mul int64
	switch unit {
	case "B":
		mul = 1
	case "KB":
		mul = 1024
	case "MB":
		mul = 1024 * 1024
	case "GB":
		mul = 1024 * 1024 * 1024
	case "TB":
		mul = 1024 * 1024 * 1024 * 1024
	default:
		return 0
	}
	return int64(value * float64(mul))
}

// removeLargeAttachment removes a large attachment by its file token.
// It updates both representations:
//
//  1. X-Lms-Large-Attachment-Ids header: removes the token from the JSON
//     ID list. If the list becomes empty, the header itself is removed.
//  2. HTML body: removes the <div id="large-file-item"> whose <a> has the
//     matching data-mail-token attribute. If the enclosing container
//     <div id="large-file-area-*"> has no remaining items, the whole
//     container is removed.
func removeLargeAttachment(snapshot *DraftSnapshot, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("remove_attachment: token is empty")
	}
	if err := removeTokenFromIDsHeader(snapshot, token); err != nil {
		return err
	}
	if err := removeTokenFromHTMLBody(snapshot, token); err != nil {
		return err
	}
	return nil
}

// removeTokenFromIDsHeader removes the given token from whichever large
// attachment header is present (CLI or server format). Returns an error
// if no header is found or the token is not listed. After removal, the
// header is re-encoded in CLI format (X-Lms-Large-Attachment-Ids) so
// the server can process the update on upload.
func removeTokenFromIDsHeader(snapshot *DraftSnapshot, token string) error {
	headerIdx := -1
	for i, h := range snapshot.Headers {
		if IsLargeAttachmentHeader(h.Name) {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return fmt.Errorf("remove_attachment: draft has no large attachment header")
	}
	items, err := decodeLargeAttachmentHeader(snapshot.Headers[headerIdx].Value)
	if err != nil {
		return fmt.Errorf("remove_attachment: malformed large attachment header: %w", err)
	}
	filtered := make([]largeAttHeaderEntry, 0, len(items))
	removed := false
	for _, it := range items {
		if it.token() == token {
			removed = true
			continue
		}
		filtered = append(filtered, it)
	}
	if !removed {
		return fmt.Errorf("remove_attachment: token %q not found in large attachment header", token)
	}
	if len(filtered) == 0 {
		snapshot.Headers = append(snapshot.Headers[:headerIdx], snapshot.Headers[headerIdx+1:]...)
		return nil
	}
	cliItems := make([]struct {
		ID string `json:"id"`
	}, len(filtered))
	for i, it := range filtered {
		cliItems[i].ID = it.token()
	}
	encoded, err := json.Marshal(cliItems)
	if err != nil {
		return fmt.Errorf("remove_attachment: failed to re-encode large attachment header: %w", err)
	}
	snapshot.Headers[headerIdx].Name = LargeAttachmentIDsHeader
	snapshot.Headers[headerIdx].Value = base64.StdEncoding.EncodeToString(encoded)
	return nil
}

// removeTokenFromHTMLBody walks the HTML body, removes the single
// large-file-item whose anchor has data-mail-token == token, and if the
// enclosing container becomes empty (no more large-file-item children),
// removes the whole container.
//
// It is not an error if the HTML body or item is missing — the header
// removal is still considered the authoritative operation. This handles
// cases where the HTML was already edited out but the header wasn't.
func removeTokenFromHTMLBody(snapshot *DraftSnapshot, token string) error {
	htmlPart := FindHTMLBodyPart(snapshot.Body)
	if htmlPart == nil || len(htmlPart.Body) == 0 {
		return nil
	}
	body := string(htmlPart.Body)
	newBody, changed := RemoveLargeFileItemFromHTML(body, token)
	if !changed {
		return nil
	}
	htmlPart.Body = []byte(newBody)
	htmlPart.Dirty = true
	return nil
}

// RemoveLargeFileItemFromHTML parses the HTML, finds the large-file-item
// containing an <a> whose token matches (via data-mail-token attribute or
// href URL token= parameter), removes that item, and if the enclosing
// large-file-area container becomes empty, removes the container as well.
// Returns the updated HTML and a changed flag.
func RemoveLargeFileItemFromHTML(htmlBody, token string) (string, bool) {
	doc, err := xhtml.Parse(strings.NewReader(htmlBody))
	if err != nil {
		return htmlBody, false
	}
	item := findLargeFileItemByToken(doc, token)
	if item == nil {
		return htmlBody, false
	}
	container := item.Parent
	// Detach the item from its parent.
	if container != nil {
		container.RemoveChild(item)
	}
	// If the container is a large-file-area and has no remaining
	// large-file-item children, remove the whole container.
	if container != nil && isLargeFileAreaContainer(container) && !hasLargeFileItemChild(container) {
		if grand := container.Parent; grand != nil {
			grand.RemoveChild(container)
		}
	}
	var buf bytes.Buffer
	if err := xhtml.Render(&buf, doc); err != nil {
		return htmlBody, false
	}
	return stripHTMLEnvelope(buf.String()), true
}

func findLargeFileItemByToken(n *xhtml.Node, token string) *xhtml.Node {
	if n == nil {
		return nil
	}
	if n.Type == xhtml.ElementNode && n.Data == "div" && attr(n, "id") == LargeFileItemID {
		if itemContainsToken(n, token) {
			return n
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findLargeFileItemByToken(c, token); found != nil {
			return found
		}
	}
	return nil
}

func itemContainsToken(item *xhtml.Node, token string) bool {
	if item == nil {
		return false
	}
	for c := item.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.ElementNode && c.Data == "a" {
			if attr(c, LargeAttachmentTokenAttr) == token {
				return true
			}
			if hrefContainsToken(attr(c, "href"), token) {
				return true
			}
		}
		if itemContainsToken(c, token) {
			return true
		}
	}
	return false
}

func hrefContainsToken(href, token string) bool {
	if href == "" || token == "" {
		return false
	}
	u, err := url.Parse(href)
	if err != nil {
		return false
	}
	return u.Query().Get("token") == token
}

func isLargeFileAreaContainer(n *xhtml.Node) bool {
	if n == nil || n.Type != xhtml.ElementNode || n.Data != "div" {
		return false
	}
	return strings.HasPrefix(attr(n, "id"), LargeFileContainerIDPrefix)
}

func hasLargeFileItemChild(n *xhtml.Node) bool {
	if n == nil {
		return false
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.ElementNode && c.Data == "div" && attr(c, "id") == LargeFileItemID {
			return true
		}
		if hasLargeFileItemChild(c) {
			return true
		}
	}
	return false
}

// stripHTMLEnvelope removes the <html><head></head><body>...</body></html>
// wrapper that xhtml.Parse + xhtml.Render adds around HTML fragments.
func stripHTMLEnvelope(s string) string {
	s = strings.TrimPrefix(s, "<html><head></head><body>")
	s = strings.TrimSuffix(s, "</body></html>")
	return s
}
