// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	html5BlockTag             = "html5-block"
	html5BlockPathAttr        = "path"
	html5BlockDataRefAttr     = "data-ref"
	html5BlockDataAttr        = "data"
	html5BlockReferenceRoot   = "doc-fetch-resources"
	html5BlockReferenceMaxRaw = 1024
)

var (
	html5BlockStartTagPattern = regexp.MustCompile(`(?is)<html5-block\b[^>]*>`)
	html5BlockElementPattern  = regexp.MustCompile(`(?is)<html5-block\b[^>]*>(.*?)</html5-block>`)
	html5BlockSafeNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

type html5BlockReferenceEntry struct {
	Data   string `json:"data,omitempty"`
	Path   string `json:"path,omitempty"`
	UserID string `json:"user_id,omitempty"`
}

type html5BlockReferenceMap map[string]map[string]html5BlockReferenceEntry

type docsV2WriteInput struct {
	Content      string
	ReferenceMap map[string]interface{}
}

type html5BlockAttr struct {
	Name  string
	Value string
}

type html5BlockStartTag struct {
	Attrs       []html5BlockAttr
	SelfClosing bool
}

func buildCreateBodyWithHTML5ReferenceMap(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	body := buildCreateBody(runtime)
	if runtime.Str("content") == "" && !runtime.Changed("reference-map") {
		return body, nil
	}
	input, err := resolveDocsV2ContentReferenceMap(runtime)
	if err != nil {
		return nil, err
	}
	body["content"] = buildCreateContentWithBody(runtime, input.Content)
	if len(input.ReferenceMap) > 0 {
		body["reference_map"] = input.ReferenceMap
	}
	return body, nil
}

func buildUpdateBodyWithHTML5ReferenceMap(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	body := buildUpdateBody(runtime)
	input, err := resolveDocsV2ContentReferenceMap(runtime)
	if err != nil {
		return nil, err
	}
	if input.Content != "" {
		body["content"] = input.Content
	}
	if len(input.ReferenceMap) > 0 {
		body["reference_map"] = input.ReferenceMap
	}
	return body, nil
}

func validateDocsV2ReferenceMapFlags(runtime *common.RuntimeContext) error {
	if runtime.Changed("reference-map") && runtime.Str("content") == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--reference-map requires --content").WithParam("--reference-map")
	}
	return nil
}

func resolveDocsV2ContentReferenceMap(runtime *common.RuntimeContext) (docsV2WriteInput, error) {
	input := docsV2WriteInput{Content: runtime.Str("content")}
	if raw := runtime.Str("reference-map"); strings.TrimSpace(raw) != "" {
		refMap, err := parseReferenceMapObject(raw, "--reference-map")
		if err != nil {
			return docsV2WriteInput{}, err
		}
		input.ReferenceMap = refMap
	}
	return prepareDocsV2WriteInput(runtime, input)
}

func prepareDocsV2WriteInput(runtime *common.RuntimeContext, input docsV2WriteInput) (docsV2WriteInput, error) {
	refMap := cloneReferenceMapObject(input.ReferenceMap)
	html5RefMap, err := html5ReferenceMapFromObject(refMap)
	if err != nil {
		return docsV2WriteInput{}, err
	}

	content, html5RefMap, err := prepareHTML5BlockWriteContent(runtime, runtime.Str("doc-format"), input.Content, html5RefMap)
	if err != nil {
		return docsV2WriteInput{}, err
	}
	if err := resolveReferenceMapPaths(runtime, html5RefMap); err != nil {
		return docsV2WriteInput{}, err
	}
	refMap = mergeHTML5ReferenceMap(refMap, html5RefMap)
	return docsV2WriteInput{
		Content:      content,
		ReferenceMap: refMap,
	}, nil
}

func parseReferenceMapObject(raw string, label string) (map[string]interface{}, error) {
	if len(bytes.TrimSpace([]byte(raw))) == 0 || string(bytes.TrimSpace([]byte(raw))) == "null" {
		return nil, nil
	}
	var refMap map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &refMap); err != nil {
		return nil, common.ValidationErrorf("%s is not valid reference_map JSON: %v", label, err).WithParam(label).WithCause(err)
	}
	return refMap, nil
}

func parseHTML5BlockReferenceMapBytes(raw []byte, label string) (html5BlockReferenceMap, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	var refMap html5BlockReferenceMap
	if err := json.Unmarshal(raw, &refMap); err != nil {
		return nil, common.ValidationErrorf("%s is not valid reference_map JSON: %v", label, err).WithParam(label).WithCause(err)
	}
	return compactReferenceMap(refMap), nil
}

func prepareHTML5BlockWriteContent(runtime *common.RuntimeContext, format string, content string, refMap html5BlockReferenceMap) (string, html5BlockReferenceMap, error) {
	if !strings.Contains(content, "<html5-block") {
		return content, compactReferenceMap(refMap), nil
	}
	if err := validateHTML5BlockWriteElementBodies(format, content); err != nil {
		return "", nil, err
	}

	refMap = cloneReferenceMap(refMap)
	if refMap == nil {
		refMap = html5BlockReferenceMap{}
	}
	ensureReferenceGroup(refMap, html5BlockTag)
	nextRef := nextHTML5BlockRef(refMap)

	rewrite := func(segment string) (string, error) {
		return rewriteHTML5BlockStartTags(segment, func(raw string) (string, error) {
			tag, err := parseHTML5BlockStartTag(raw)
			if err != nil {
				return "", common.ValidationErrorf("invalid html5-block tag: %v", err).WithParam("html5-block")
			}
			if tag.hasAttr(html5BlockDataAttr) {
				return "", common.ValidationErrorf("html5-block data is reserved for SDK internals; use data-ref with reference_map or path=\"@relative.html\"").WithParam("html5-block")
			}

			pathValue, hasPath := tag.attr(html5BlockPathAttr)
			dataRef, hasDataRef := tag.attr(html5BlockDataRefAttr)
			if hasPath && hasDataRef {
				return "", common.ValidationErrorf("html5-block cannot contain both path and data-ref").WithParam("html5-block")
			}
			if hasDataRef {
				ref := strings.TrimSpace(dataRef)
				if ref == "" {
					return "", common.ValidationErrorf("html5-block data-ref cannot be empty").WithParam("data-ref")
				}
				if _, ok := refMap[html5BlockTag][ref]; !ok {
					return "", common.ValidationErrorf("reference_map.%s.%s is required for html5-block data-ref", html5BlockTag, ref).WithParam("reference_map")
				}
				return tag.render(false), nil
			}
			if !hasPath {
				return "", common.ValidationErrorf("html5-block requires path=\"@relative.html\" or data-ref with reference_map").WithParam("html5-block")
			}

			data, err := readHTML5BlockPath(runtime, pathValue, "html5-block path")
			if err != nil {
				return "", err
			}
			ref := nextRef()
			refMap[html5BlockTag][ref] = html5BlockReferenceEntry{Data: data}
			tag.removeAttrs(html5BlockPathAttr, html5BlockDataRefAttr, html5BlockDataAttr)
			tag.Attrs = append(tag.Attrs, html5BlockAttr{Name: html5BlockDataRefAttr, Value: ref})
			return tag.render(false), nil
		})
	}

	var (
		out string
		err error
	)
	if strings.TrimSpace(format) == "markdown" {
		out = applyOutsideCodeFences(content, func(segment string) string {
			if err != nil {
				return segment
			}
			outSegment, rewriteErr := rewrite(segment)
			if rewriteErr != nil {
				err = rewriteErr
				return segment
			}
			return outSegment
		})
	} else {
		out, err = rewrite(content)
	}
	if err != nil {
		return "", nil, err
	}
	return out, compactReferenceMap(refMap), nil
}

func validateHTML5BlockWriteElementBodies(format string, content string) error {
	validateSegment := func(segment string) error {
		matches := html5BlockElementPattern.FindAllStringSubmatchIndex(segment, -1)
		for _, match := range matches {
			if len(match) < 4 || match[2] < 0 || match[3] < 0 {
				continue
			}
			if strings.TrimSpace(segment[match[2]:match[3]]) != "" {
				return common.ValidationErrorf("html5-block content must be loaded from path=\"@relative.html\" or reference_map; remove content between <html5-block> and </html5-block>").WithParam("html5-block")
			}
		}
		return nil
	}

	if strings.TrimSpace(format) != "markdown" {
		return validateSegment(content)
	}

	var validateErr error
	_ = applyOutsideCodeFences(content, func(segment string) string {
		if validateErr != nil {
			return segment
		}
		validateErr = validateSegment(segment)
		return segment
	})
	return validateErr
}

func processHTML5BlockReferenceMapForFetch(runtime *common.RuntimeContext, format string, docToken string, data map[string]interface{}) error {
	doc, _ := data["document"].(map[string]interface{})
	if doc == nil {
		return nil
	}
	content, _ := doc["content"].(string)
	if !hasProcessableHTML5Block(format, content) {
		return nil
	}

	refMap, err := referenceMapFromDocument(doc)
	if err != nil {
		return err
	}
	group := refMap[html5BlockTag]
	if group == nil {
		return common.ValidationErrorf("document.reference_map.%s is required for fetched html5-block content", html5BlockTag).WithParam("reference_map")
	}

	if err := validateFetchedHTML5BlockRefs(format, content, refMap); err != nil {
		return err
	}

	changed := false
	for ref, entry := range group {
		if entry.Data == "" || len([]byte(entry.Data)) <= html5BlockReferenceMaxRaw {
			continue
		}
		relPath, err := writeHTML5BlockReferenceFile(runtime, docToken, ref, entry.Data)
		if err != nil {
			return err
		}
		entry.Data = ""
		entry.Path = "@" + filepath.ToSlash(relPath)
		group[ref] = entry
		changed = true
	}
	if changed {
		doc["reference_map"] = refMap
	}
	return nil
}

func referenceMapFromDocument(doc map[string]interface{}) (html5BlockReferenceMap, error) {
	raw, ok := doc["reference_map"]
	if !ok || raw == nil {
		return nil, common.ValidationErrorf("document.reference_map is required for fetched html5-block content").WithParam("reference_map")
	}
	refMap, err := referenceMapFromValue(raw, "document.reference_map")
	if err != nil {
		return nil, err
	}
	if len(refMap) == 0 {
		return nil, common.ValidationErrorf("document.reference_map is required for fetched html5-block content").WithParam("reference_map")
	}
	return refMap, nil
}

func referenceMapFromValue(value interface{}, label string) (html5BlockReferenceMap, error) {
	if typed, ok := value.(html5BlockReferenceMap); ok {
		return compactReferenceMap(typed), nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, common.ValidationErrorf("%s is not valid reference_map JSON: %v", label, err).WithParam("reference_map").WithCause(err)
	}
	return parseHTML5BlockReferenceMapBytes(raw, label)
}

func validateFetchedHTML5BlockRefs(format string, content string, refMap html5BlockReferenceMap) error {
	validateSegment := func(segment string) error {
		_, err := rewriteHTML5BlockStartTags(segment, func(raw string) (string, error) {
			tag, parseErr := parseHTML5BlockStartTag(raw)
			if parseErr != nil {
				return raw, common.ValidationErrorf("invalid html5-block tag in fetched content: %v", parseErr).WithParam("html5-block")
			}
			ref, ok := tag.attr(html5BlockDataRefAttr)
			if !ok || strings.TrimSpace(ref) == "" {
				return raw, common.ValidationErrorf("fetched html5-block is missing data-ref; cannot resolve HTML reference").WithParam("html5-block")
			}
			ref = strings.TrimSpace(ref)
			if _, ok := refMap[html5BlockTag][ref]; !ok {
				return raw, common.ValidationErrorf("document.reference_map.%s.%s is missing; cannot resolve html5-block. Re-run fetch or check that the upstream document.reference_map field includes this ref.", html5BlockTag, ref).WithParam("reference_map")
			}
			return raw, nil
		})
		return err
	}

	if strings.TrimSpace(format) != "markdown" {
		return validateSegment(content)
	}
	var validateErr error
	_ = applyOutsideCodeFences(content, func(segment string) string {
		if validateErr != nil {
			return segment
		}
		validateErr = validateSegment(segment)
		return segment
	})
	return validateErr
}

func resolveReferenceMapPaths(runtime *common.RuntimeContext, refMap html5BlockReferenceMap) error {
	for typ, group := range refMap {
		for ref, entry := range group {
			if strings.TrimSpace(entry.Path) == "" {
				continue
			}
			if entry.Data != "" {
				return common.ValidationErrorf("reference_map.%s.%s must use either data or path, not both", typ, ref).WithParam("reference_map")
			}
			data, err := readHTML5BlockPath(runtime, entry.Path, fmt.Sprintf("reference_map.%s.%s.path", typ, ref))
			if err != nil {
				return err
			}
			entry.Data = data
			entry.Path = ""
			group[ref] = entry
		}
	}
	return nil
}

func readHTML5BlockPath(runtime *common.RuntimeContext, pathValue string, label string) (string, error) {
	pathRaw := strings.TrimSpace(pathValue)
	if !strings.HasPrefix(pathRaw, "@") {
		return "", common.ValidationErrorf("%s %q must start with @, for example @widget.html", label, pathValue).WithParam("path")
	}
	relPath := strings.TrimSpace(strings.TrimPrefix(pathRaw, "@"))
	if relPath == "" {
		return "", common.ValidationErrorf("%s cannot be empty after @", label).WithParam("path")
	}
	clean := filepath.Clean(relPath)
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", common.ValidationErrorf("%s %q must be a relative path within the current working directory", label, pathValue).WithParam("path")
	}
	if strings.ToLower(filepath.Ext(clean)) != ".html" {
		return "", common.ValidationErrorf("%s %q must point to a .html file", label, pathValue).WithParam("path")
	}
	data, err := cmdutil.ReadInputFile(runtime.FileIO(), clean)
	if err != nil {
		return "", common.ValidationErrorf("%s %q cannot be read from the current working directory; check that the file exists relative to where lark-cli is running: %v", label, clean, err).WithParam("path").WithCause(err)
	}
	return string(data), nil
}

func hasProcessableHTML5Block(format string, content string) bool {
	if !strings.Contains(content, "<html5-block") {
		return false
	}
	if strings.TrimSpace(format) != "markdown" {
		return true
	}
	found := false
	_ = applyOutsideCodeFences(content, func(segment string) string {
		if strings.Contains(segment, "<html5-block") {
			found = true
		}
		return segment
	})
	return found
}

func applyOutsideCodeFences(content string, fn func(segment string) string) string {
	var out strings.Builder
	var segment strings.Builder
	inFence := false

	flush := func() {
		if segment.Len() == 0 {
			return
		}
		out.WriteString(fn(segment.String()))
		segment.Reset()
	}

	for _, line := range strings.SplitAfter(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if !inFence {
				flush()
				inFence = true
			} else {
				inFence = false
			}
			out.WriteString(line)
			continue
		}
		if inFence {
			out.WriteString(line)
		} else {
			segment.WriteString(line)
		}
	}
	flush()
	return out.String()
}

func cloneReferenceMap(refMap html5BlockReferenceMap) html5BlockReferenceMap {
	if len(refMap) == 0 {
		return nil
	}
	out := make(html5BlockReferenceMap, len(refMap))
	for typ, group := range refMap {
		if len(group) == 0 {
			continue
		}
		outGroup := make(map[string]html5BlockReferenceEntry, len(group))
		for ref, entry := range group {
			outGroup[ref] = entry
		}
		out[typ] = outGroup
	}
	return out
}

func cloneReferenceMapObject(refMap map[string]interface{}) map[string]interface{} {
	if len(refMap) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(refMap))
	for key, value := range refMap {
		out[key] = value
	}
	return out
}

func html5ReferenceMapFromObject(refMap map[string]interface{}) (html5BlockReferenceMap, error) {
	if len(refMap) == 0 {
		return nil, nil
	}
	group, ok := refMap[html5BlockTag]
	if !ok || group == nil {
		return nil, nil
	}
	return referenceMapFromValue(map[string]interface{}{html5BlockTag: group}, "reference_map."+html5BlockTag)
}

func mergeHTML5ReferenceMap(refMap map[string]interface{}, html5RefMap html5BlockReferenceMap) map[string]interface{} {
	group := html5RefMap[html5BlockTag]
	if len(group) == 0 {
		return refMap
	}
	if refMap == nil {
		refMap = map[string]interface{}{}
	}
	refMap[html5BlockTag] = group
	return refMap
}

func compactReferenceMap(refMap html5BlockReferenceMap) html5BlockReferenceMap {
	if len(refMap) == 0 {
		return nil
	}
	out := make(html5BlockReferenceMap, len(refMap))
	for typ, group := range refMap {
		if len(group) == 0 {
			continue
		}
		out[typ] = group
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func ensureReferenceGroup(refMap html5BlockReferenceMap, typ string) {
	if refMap[typ] == nil {
		refMap[typ] = map[string]html5BlockReferenceEntry{}
	}
}

func nextHTML5BlockRef(refMap html5BlockReferenceMap) func() string {
	next := 1
	return func() string {
		for {
			ref := fmt.Sprintf("html5_%d", next)
			next++
			if _, exists := refMap[html5BlockTag][ref]; !exists {
				return ref
			}
		}
	}
}

func writeHTML5BlockReferenceFile(runtime *common.RuntimeContext, docToken string, ref string, html string) (string, error) {
	if !isSafeHTML5BlockResourceName(docToken) {
		return "", common.ValidationErrorf("document_id %q cannot be used as a resource directory name", docToken).WithParam("document_id")
	}
	if !isSafeHTML5BlockResourceName(ref) {
		return "", common.ValidationErrorf("html5-block data-ref %q cannot be used as a file name", ref).WithParam("data-ref")
	}
	relPath := filepath.Join(html5BlockReferenceRoot, docToken, ref+".html")
	data := []byte(html)
	_, err := runtime.FileIO().Save(relPath, fileio.SaveOptions{
		ContentType:   "text/html; charset=utf-8",
		ContentLength: int64(len(data)),
	}, bytes.NewReader(data))
	if err != nil {
		if errors.Is(err, fileio.ErrPathValidation) {
			return "", common.ValidationErrorf("cannot write html5-block reference file %q: %v", relPath, err).WithParam("reference_map").WithCause(err)
		}
		return "", errs.NewInternalError(errs.SubtypeFileIO, "cannot write html5-block reference file %q: %v", relPath, err).WithCause(err)
	}
	return relPath, nil
}

func isSafeHTML5BlockResourceName(name string) bool {
	return name != "." && name != ".." && html5BlockSafeNamePattern.MatchString(name)
}

func rewriteHTML5BlockStartTags(content string, fn func(raw string) (string, error)) (string, error) {
	var rewriteErr error
	out := html5BlockStartTagPattern.ReplaceAllStringFunc(content, func(raw string) string {
		if rewriteErr != nil {
			return raw
		}
		rewritten, err := fn(raw)
		if err != nil {
			rewriteErr = err
			return raw
		}
		return rewritten
	})
	if rewriteErr != nil {
		return "", rewriteErr
	}
	return out, nil
}

func parseHTML5BlockStartTag(raw string) (html5BlockStartTag, error) {
	trimmed := strings.TrimSpace(raw)
	selfClosing := strings.HasSuffix(trimmed, "/>")
	decoder := xml.NewDecoder(strings.NewReader(raw))
	for {
		tok, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return html5BlockStartTag{}, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != html5BlockTag {
			return html5BlockStartTag{}, fmt.Errorf("expected <%s>, got <%s>", html5BlockTag, start.Name.Local) //nolint:forbidigo // intermediate parse helper; callers wrap with typed validation errors.
		}
		attrs := make([]html5BlockAttr, 0, len(start.Attr))
		for _, attr := range start.Attr {
			attrs = append(attrs, html5BlockAttr{Name: attr.Name.Local, Value: attr.Value})
		}
		return html5BlockStartTag{Attrs: attrs, SelfClosing: selfClosing}, nil
	}
	return html5BlockStartTag{}, fmt.Errorf("missing start element") //nolint:forbidigo // intermediate parse helper; callers wrap with typed validation errors.
}

func (t html5BlockStartTag) attr(name string) (string, bool) {
	for _, attr := range t.Attrs {
		if attr.Name == name {
			return attr.Value, true
		}
	}
	return "", false
}

func (t html5BlockStartTag) hasAttr(name string) bool {
	_, ok := t.attr(name)
	return ok
}

func (t *html5BlockStartTag) removeAttrs(names ...string) {
	remove := make(map[string]struct{}, len(names))
	for _, name := range names {
		remove[name] = struct{}{}
	}
	attrs := t.Attrs[:0]
	for _, attr := range t.Attrs {
		if _, ok := remove[attr.Name]; ok {
			continue
		}
		attrs = append(attrs, attr)
	}
	t.Attrs = attrs
}

func (t html5BlockStartTag) render(selfClosing bool) string {
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(html5BlockTag)
	for _, attr := range t.Attrs {
		b.WriteByte(' ')
		b.WriteString(attr.Name)
		b.WriteString(`="`)
		b.WriteString(escapeXMLAttr(attr.Value))
		b.WriteByte('"')
	}
	if selfClosing {
		b.WriteString("/>")
	} else {
		b.WriteByte('>')
	}
	if t.SelfClosing && !selfClosing {
		b.WriteString("</")
		b.WriteString(html5BlockTag)
		b.WriteByte('>')
	}
	return b.String()
}

func escapeXMLAttr(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
