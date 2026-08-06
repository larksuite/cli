// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// FetchAnchoredMarkdown reads a Doc or Docx URL as Markdown with block anchors.
func FetchAnchoredMarkdown(ctx context.Context, runtime *common.RuntimeContext, rawURL string, opts FetchOptions) (*FetchResult, error) {
	req := NewRequest(rawURL)
	req.WithBlockID = true
	ApplyPagination(&req, opts.Full, opts.PageToken, opts.PageSize)
	resp, err := FetchDocInfo(ctx, runtime, req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"document read returned an empty response")
	}
	if strings.TrimSpace(resp.FullContent) == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"document read returned no anchored content")
	}
	md, rerr := RenderAnchoredMarkdown(resp, opts.MaxRows)
	if rerr != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"could not render anchored Markdown: %v", rerr).WithCause(rerr)
	}
	if strings.TrimSpace(md) == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"document read rendered no Markdown content")
	}
	return &FetchResult{
		Content:       md,
		Title:         resp.Title,
		UpdateTime:    resp.UpdateTime,
		HasMore:       resp.HasMore,
		NextPageToken: resp.NextPageToken,
	}, nil
}

// RenderAnchoredMarkdown converts anchored XML to readable Markdown and limits
// materialized tables to maxRows.
func RenderAnchoredMarkdown(resp *Response, maxRows int) (string, error) {
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return "", nil
	}
	return renderAnchoredMarkdown(resp.FullContent, resp.ImageMetaMap, maxRows)
}

var anchoredMarkdownBlankRunRe = regexp.MustCompile(`\n{3,}`)

func renderAnchoredMarkdown(xmlContent string, metas map[string]*ImageMeta, maxRows int) (string, error) {
	r := &anchoredMarkdownRenderer{metas: metas}
	// Content-read returns an XML fragment with occasional HTML constructs and
	// unescaped text, so normalize it and decode under a synthetic root.
	dec := xml.NewDecoder(strings.NewReader("<contentroot>" + escapeBareLessThan(stripInvalidXMLChars(xmlContent)) + "</contentroot>"))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	if err := r.renderChildren(dec, ""); err != nil {
		return "", err
	}
	md := anchoredMarkdownBlankRunRe.ReplaceAllString(r.out.String(), "\n\n")
	md = TruncateGFMTables(md, maxRows, "")
	return strings.TrimSpace(md) + "\n", nil
}

type anchoredMarkdownRenderer struct {
	metas map[string]*ImageMeta
	out   strings.Builder
}

func (r *anchoredMarkdownRenderer) renderChildren(dec *xml.Decoder, parentName string) error {
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if err := r.renderBlock(dec, t); err != nil {
				return err
			}
		case xml.EndElement:
			if parentName != "" && t.Name.Local == parentName {
				return nil
			}
		}
	}
}

func (r *anchoredMarkdownRenderer) renderBlock(dec *xml.Decoder, start xml.StartElement) error {
	name := start.Name.Local
	switch {
	case isHeadingTag(name):
		level := int(name[1] - '0')
		txt, err := r.readText(dec, name)
		if err != nil {
			return err
		}
		if txt = normalizeInline(txt); txt != "" {
			r.out.WriteString(strings.Repeat("#", level) + " " + txt + idSuffix(realID(start)) + "\n\n")
		}
		return nil
	case name == "p":
		txt, err := r.readText(dec, name)
		if err != nil {
			return err
		}
		if txt = normalizeInline(txt); txt != "" {
			r.out.WriteString(txt + "\n\n")
		}
		return nil
	case name == "ul" || name == "ol":
		return r.renderList(dec, start)
	case name == "pre":
		return r.renderCode(dec, start)
	case name == "img":
		r.out.WriteString(r.renderImg(start) + "\n\n")
		return dec.Skip()
	case name == "sheet" || name == "bitable" || name == "synced" || name == "component":
		return r.renderEmbedTable(dec, start)
	case name == "whiteboard" || name == "board":
		r.out.WriteString("> " + resTokenLink("画板", attrOf(start, "token")) + idSuffix(realID(start)) + "\n\n")
		return dec.Skip()
	default:
		// Unknown wrapper (including the synthetic contentroot): descend into children.
		return r.renderChildren(dec, name)
	}
}

func (r *anchoredMarkdownRenderer) renderList(dec *xml.Decoder, start xml.StartElement) error {
	ordered := start.Name.Local == "ol"
	n := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "li" {
				txt, err := r.readText(dec, "li")
				if err != nil {
					return err
				}
				if txt = normalizeInline(txt); txt != "" {
					n++
					marker := "- "
					if ordered {
						marker = strconv.Itoa(n) + ". "
					}
					r.out.WriteString(marker + txt + "\n")
				}
			} else {
				// Content-read may interleave embedded blocks between list items.
				r.out.WriteString("\n")
				if err := r.renderBlock(dec, t); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				r.out.WriteString("\n")
				return nil
			}
		}
	}
}

func (r *anchoredMarkdownRenderer) renderCode(dec *xml.Decoder, start xml.StartElement) error {
	lang := attrOf(start, "lang")
	code, err := r.readText(dec, "pre")
	if err != nil {
		return err
	}
	r.out.WriteString("```" + lang + "\n" + strings.Trim(code, "\n") + "\n```\n\n")
	return nil
}

func (r *anchoredMarkdownRenderer) renderImg(start xml.StartElement) string {
	token := attrOf(start, "token")
	base := RenderOneImage(token, r.metas[token])
	return base + idSuffix(realID(start))
}

// renderEmbedTable handles table-shaped embeds, text-shaped embeds, and
// placeholders when the service did not materialize either form.
func (r *anchoredMarkdownRenderer) renderEmbedTable(dec *xml.Decoder, start xml.StartElement) error {
	id := realID(start)
	token := attrOf(start, "token")
	rows, rawText, err := r.collectRows(dec, start.Name.Local)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		r.out.WriteString("**" + resTokenLink("表", token) + "**" + idSuffix(id) + "\n\n")
		r.out.WriteString(rowsToGFM(rows))
		r.out.WriteString("\n")
		return nil
	}
	if md := strings.TrimSpace(rawText); md != "" {
		r.out.WriteString("**" + resTokenLink(embedLabel(start.Name.Local), token) + "**" + idSuffix(id) + "\n\n")
		r.out.WriteString(md)
		r.out.WriteString("\n")
		return nil
	}
	r.out.WriteString("**" + resTokenLink(embedLabel(start.Name.Local), token) + "**" + idSuffix(id) + "\n")
	r.out.WriteString("> 内容可能未展开" + embedSkillHint(start.Name.Local) + "\n\n")
	return nil
}

func embedLabel(tag string) string {
	switch tag {
	case "sheet":
		return "电子表格"
	case "bitable":
		return "多维表格"
	case "synced":
		return "同步块"
	case "component":
		return "引用内容"
	default:
		return "嵌入内容"
	}
}

func embedSkillHint(tag string) string {
	switch tag {
	case "sheet":
		return "，用 sheets +cells-get 取"
	case "bitable":
		return "，用 base +record-list 取"
	default:
		return ""
	}
}

// collectRows supports both nested HTML tables and text-only embeds.
func (r *anchoredMarkdownRenderer) collectRows(dec *xml.Decoder, name string) (rows [][]string, rawText string, err error) {
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return rows, text.String(), nil
		}
		if err != nil {
			return rows, text.String(), err
		}
		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.StartElement:
			if t.Name.Local == "tr" {
				cells, err := r.collectCells(dec)
				if err != nil {
					return rows, text.String(), err
				}
				rows = append(rows, cells)
			}
		case xml.EndElement:
			if t.Name.Local == name {
				return rows, text.String(), nil
			}
		}
	}
}

func (r *anchoredMarkdownRenderer) collectCells(dec *xml.Decoder) ([]string, error) {
	var cells []string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return cells, nil
		}
		if err != nil {
			return cells, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "td" || t.Name.Local == "th" {
				txt, err := r.readText(dec, t.Name.Local)
				if err != nil {
					return cells, err
				}
				cells = append(cells, gfmCell(r.renderCellImages(txt)))
			}
		case xml.EndElement:
			if t.Name.Local == "tr" {
				return cells, nil
			}
		}
	}
}

func (r *anchoredMarkdownRenderer) readText(dec *xml.Decoder, name string) (string, error) {
	var b strings.Builder
	depth := 1
	for {
		tok, err := dec.Token()
		if err != nil {
			return b.String(), err
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				return b.String(), nil
			}
		}
	}
}

func isHeadingTag(name string) bool {
	return len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6'
}

func realID(start xml.StartElement) string {
	id := attrOf(start, "id")
	if id == "" || isAllDigits(id) {
		return ""
	}
	return id
}

func idSuffix(id string) string {
	if id == "" {
		return ""
	}
	return " {#" + id + "}"
}

// resTokenLink keeps resource tokens distinct from block anchors.
func resTokenLink(label, token string) string {
	if token == "" {
		return label
	}
	return "[" + label + "](token=" + token + ")"
}

func attrOf(start xml.StartElement, name string) string {
	for _, a := range start.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isInvalidXMLChar(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	return r < 0x20 || r == 0xFFFE || r == 0xFFFF
}

// stripInvalidXMLChars removes controls rejected by encoding/xml.
func stripInvalidXMLChars(s string) string {
	if strings.IndexFunc(s, isInvalidXMLChar) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isInvalidXMLChar(r) {
			return -1
		}
		return r
	}, s)
}

// escapeBareLessThan preserves unescaped '<' text without masking real tags.
func escapeBareLessThan(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '<' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 < len(s) && plausibleTagStart(s[i+1]) {
			b.WriteByte('<')
			continue
		}
		b.WriteString("&lt;")
	}
	return b.String()
}

func plausibleTagStart(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':
		return true
	case c == '_' || c == ':' || c == '/' || c == '!' || c == '?':
		return true
	}
	return false
}

func normalizeInline(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}
