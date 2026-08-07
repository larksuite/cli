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

	whiteboardTag      = "whiteboard"
	whiteboardTypeAttr = "type"
	whiteboardPathAttr = "path"

	isvBlockTag       = "isv-block"
	isvBlockTypeAttr  = "type"
	isvBlockRefPrefix = "isv"
)

var (
	html5BlockStartTagPattern = regexp.MustCompile(`(?is)<html5-block\b[^>]*>`)
	html5BlockElementPattern  = regexp.MustCompile(`(?is)<html5-block\b[^>]*>(.*?)</html5-block>`)
	isvBlockStartTagPattern   = regexp.MustCompile(`(?is)<isv-block\b[^>]*>`)
	isvBlockElementPattern    = regexp.MustCompile(`(?is)<isv-block\b[^>]*>(.*?)</isv-block>`)
	html5BlockSafeNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	whiteboardElementPattern  = regexp.MustCompile(`(?is)<whiteboard\b[^>]*(?:/>|>.*?</whiteboard>)`)
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

type whiteboardStartTag struct {
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

	content, err := prepareWhiteboardWriteContent(runtime, runtime.Str("doc-format"), input.Content)
	if err != nil {
		return docsV2WriteInput{}, err
	}
	content, html5RefMap, err = prepareHTML5BlockWriteContent(runtime, runtime.Str("doc-format"), content, html5RefMap)
	if err != nil {
		return docsV2WriteInput{}, err
	}
	if err := resolveReferenceMapPaths(runtime, html5RefMap); err != nil {
		return docsV2WriteInput{}, err
	}
	refMap = mergeHTML5ReferenceMap(refMap, html5RefMap)

	isvRefMap, err := isvReferenceMapFromObject(refMap)
	if err != nil {
		return docsV2WriteInput{}, err
	}
	content, isvRefMap, err = prepareISVBlockWriteContent(runtime, runtime.Str("doc-format"), content, isvRefMap)
	if err != nil {
		return docsV2WriteInput{}, err
	}
	if err := resolveISVReferenceMapPaths(runtime, isvRefMap); err != nil {
		return docsV2WriteInput{}, err
	}
	refMap = mergeISVReferenceMap(refMap, isvRefMap)
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

func prepareWhiteboardWriteContent(runtime *common.RuntimeContext, format string, content string) (string, error) {
	if !strings.Contains(content, "<whiteboard") {
		return content, nil
	}

	rewrite := func(segment string) (string, error) {
		return rewriteWhiteboardFileRefs(runtime, segment)
	}

	if strings.TrimSpace(format) != "markdown" {
		return rewrite(content)
	}

	var rewriteErrs []error
	out := applyOutsideCodeFences(content, func(segment string) string {
		outSegment, rewriteErr := rewrite(segment)
		if rewriteErr != nil {
			rewriteErrs = append(rewriteErrs, rewriteErr)
			return segment
		}
		return outSegment
	})
	if len(rewriteErrs) > 0 {
		return "", aggregateWhiteboardRewriteErrors(rewriteErrs)
	}
	return out, nil
}

func rewriteWhiteboardFileRefs(runtime *common.RuntimeContext, content string) (string, error) {
	var rewriteErrs []error
	out := whiteboardElementPattern.ReplaceAllStringFunc(content, func(raw string) string {
		rewritten, err := rewriteWhiteboardFileRef(runtime, raw)
		if err != nil {
			rewriteErrs = append(rewriteErrs, err)
			return raw
		}
		return rewritten
	})
	if len(rewriteErrs) > 0 {
		return "", aggregateWhiteboardRewriteErrors(rewriteErrs)
	}
	return out, nil
}

func rewriteWhiteboardFileRef(runtime *common.RuntimeContext, raw string) (string, error) {
	startRaw, body, _, ok := splitWhiteboardElement(raw)
	if !ok {
		return raw, nil
	}
	tag, err := parseWhiteboardStartTag(startRaw)
	if err != nil {
		return "", common.ValidationErrorf("invalid whiteboard tag: %v", err).WithParam("whiteboard")
	}

	pathValue, hasPath := tag.attr(whiteboardPathAttr)
	bodyPath, hasBodyPath := whiteboardBodyPathRef(body)
	if !hasPath && !hasBodyPath {
		return raw, nil
	}
	if hasPath && strings.TrimSpace(body) != "" {
		return "", common.ValidationErrorf("whiteboard cannot contain both path and inline content").WithParam("whiteboard")
	}
	if hasPath && hasBodyPath {
		return "", common.ValidationErrorf("whiteboard cannot contain both path and @file body").WithParam("whiteboard")
	}

	typRaw, ok := tag.attr(whiteboardTypeAttr)
	if !ok || strings.TrimSpace(typRaw) == "" {
		return "", common.ValidationErrorf("whiteboard file input requires type=\"svg\", type=\"mermaid\", or type=\"plantuml\"").WithParam("type")
	}
	typ, ok := canonicalWhiteboardFileType(typRaw)
	if !ok {
		return "", common.ValidationErrorf("whiteboard file input only supports type=\"svg\", type=\"mermaid\", or type=\"plantuml\", got %q", typRaw).WithParam("type")
	}

	if hasBodyPath {
		pathValue = bodyPath
	}
	data, err := readWhiteboardPath(runtime, pathValue, typ)
	if err != nil {
		return "", err
	}

	tag.setAttr(whiteboardTypeAttr, typ)
	tag.removeAttrs(whiteboardPathAttr)
	return tag.render(false) + whiteboardContentForType(typ, data) + "</" + whiteboardTag + ">", nil
}

func splitWhiteboardElement(raw string) (startTag string, body string, selfClosing bool, ok bool) {
	trimmed := strings.TrimSpace(raw)
	selfClosing = strings.HasSuffix(trimmed, "/>")
	if selfClosing {
		return raw, "", true, true
	}
	startEnd := strings.Index(raw, ">")
	if startEnd < 0 {
		return "", "", false, false
	}
	endStart := strings.LastIndex(strings.ToLower(raw), "</whiteboard>")
	if endStart < 0 || endStart < startEnd {
		return "", "", false, false
	}
	return raw[:startEnd+1], raw[startEnd+1 : endStart], false, true
}

func whiteboardBodyPathRef(body string) (string, bool) {
	trimmed := strings.TrimSpace(body)
	if !strings.HasPrefix(trimmed, "@") || strings.HasPrefix(trimmed, "@@") {
		return "", false
	}
	if strings.ContainsAny(trimmed, "\r\n") {
		return "", false
	}
	return trimmed, true
}

func canonicalWhiteboardFileType(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "svg":
		return "svg", true
	case "mermaid":
		return "mermaid", true
	case "plantuml":
		return "plantuml", true
	default:
		return "", false
	}
}

func readWhiteboardPath(runtime *common.RuntimeContext, pathValue string, typ string) (string, error) {
	pathRaw := strings.TrimSpace(pathValue)
	if !strings.HasPrefix(pathRaw, "@") {
		return "", common.ValidationErrorf("whiteboard %s path %q must start with @, for example @diagram.%s", typ, pathValue, exampleWhiteboardExt(typ)).WithParam("path")
	}
	relPath := strings.TrimSpace(strings.TrimPrefix(pathRaw, "@"))
	if relPath == "" {
		return "", common.ValidationErrorf("whiteboard %s path cannot be empty after @", typ).WithParam("path")
	}
	clean := filepath.Clean(relPath)
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", common.ValidationErrorf("whiteboard %s path %q must be a relative path within the current working directory", typ, pathValue).WithParam("path")
	}
	if !whiteboardExtAllowed(typ, strings.ToLower(filepath.Ext(clean))) {
		return "", common.ValidationErrorf("whiteboard %s path %q must point to a %s file", typ, pathValue, whiteboardExtList(typ)).WithParam("path")
	}
	data, err := cmdutil.ReadInputFile(runtime.FileIO(), clean)
	if err != nil {
		return "", common.ValidationErrorf("whiteboard %s path %q cannot be read from the current working directory; check that the file exists relative to where lark-cli is running: %v", typ, clean, err).
			WithParam("path").
			WithParams(errs.InvalidParam{Name: clean, Reason: fmt.Sprintf("whiteboard %s path cannot be read", typ)}).
			WithCause(err)
	}
	return string(data), nil
}

func whiteboardExtAllowed(typ string, ext string) bool {
	for _, allowed := range whiteboardAllowedExts(typ) {
		if ext == allowed {
			return true
		}
	}
	return false
}

func whiteboardAllowedExts(typ string) []string {
	switch typ {
	case "svg":
		return []string{".svg"}
	case "mermaid":
		return []string{".mermaid", ".mmd"}
	case "plantuml":
		return []string{".plantuml", ".puml", ".pu", ".uml"}
	default:
		return nil
	}
}

func whiteboardExtList(typ string) string {
	return strings.Join(whiteboardAllowedExts(typ), ", ")
}

func exampleWhiteboardExt(typ string) string {
	exts := whiteboardAllowedExts(typ)
	if len(exts) == 0 {
		return "txt"
	}
	return strings.TrimPrefix(exts[0], ".")
}

func whiteboardContentForType(typ string, data string) string {
	if typ == "svg" {
		return data
	}
	return escapeXMLText(data)
}

func aggregateWhiteboardRewriteErrors(rewriteErrs []error) error {
	flatErrs := flattenWhiteboardRewriteErrors(rewriteErrs)
	messages := make([]string, 0, len(flatErrs))
	params := make([]errs.InvalidParam, 0, len(flatErrs))
	for _, err := range flatErrs {
		messages = append(messages, err.Error())
		params = append(params, whiteboardInvalidParamsFromError(err)...)
	}
	validationErr := common.ValidationErrorf("whiteboard file input failed: %s", strings.Join(messages, "; ")).
		WithParam("whiteboard").
		WithCause(errors.Join(flatErrs...))
	if len(params) > 0 {
		validationErr.WithParams(params...)
	}
	return validationErr
}

func flattenWhiteboardRewriteErrors(rewriteErrs []error) []error {
	flatErrs := make([]error, 0, len(rewriteErrs))
	for _, err := range rewriteErrs {
		var validationErr *errs.ValidationError
		if errors.As(err, &validationErr) && validationErr.Param == "whiteboard" && validationErr.Cause != nil {
			if joined, ok := validationErr.Cause.(interface{ Unwrap() []error }); ok {
				flatErrs = append(flatErrs, flattenWhiteboardRewriteErrors(joined.Unwrap())...)
				continue
			}
		}
		flatErrs = append(flatErrs, err)
	}
	return flatErrs
}

func whiteboardInvalidParamsFromError(err error) []errs.InvalidParam {
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		return nil
	}
	if len(validationErr.Params) > 0 {
		return validationErr.Params
	}
	if validationErr.Param != "" {
		return []errs.InvalidParam{{Name: validationErr.Param, Reason: validationErr.Message}}
	}
	return nil
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

func prepareISVBlockWriteContent(runtime *common.RuntimeContext, format string, content string, refMap html5BlockReferenceMap) (string, html5BlockReferenceMap, error) {
	if !strings.Contains(content, "<isv-block") {
		return content, compactReferenceMap(refMap), nil
	}
	if err := validateISVBlockWriteElementBodies(format, content); err != nil {
		return "", nil, err
	}

	refMap = cloneReferenceMap(refMap)
	if refMap == nil {
		refMap = html5BlockReferenceMap{}
	}
	ensureReferenceGroup(refMap, isvBlockTag)
	nextRef := nextISVBlockRef(refMap)

	rewrite := func(segment string) (string, error) {
		return rewriteISVBlockStartTags(segment, func(raw string) (string, error) {
			tag, err := parseISVBlockStartTag(raw)
			if err != nil {
				return "", common.ValidationErrorf("invalid isv-block type tag for reference_map.isv-block.<ref>: %v", err).WithParam("isv-block")
			}
			if !tag.hasAttr(isvBlockTypeAttr) {
				return "", common.ValidationErrorf("isv-block requires type attribute before using path or reference_map.isv-block.<ref>").WithParam("type")
			}
			if tag.hasAttr(html5BlockDataAttr) {
				return "", common.ValidationErrorf("isv-block type data is reserved for SDK internals; use data-ref with reference_map.isv-block.<ref> or path=\"@relative.data\"").WithParam("isv-block")
			}

			pathValue, hasPath := tag.attr(html5BlockPathAttr)
			dataRef, hasDataRef := tag.attr(html5BlockDataRefAttr)
			if hasPath && hasDataRef {
				return "", common.ValidationErrorf("isv-block type cannot contain both path and data-ref; use either path or reference_map.isv-block.<ref>").WithParam("isv-block")
			}
			if hasDataRef {
				ref := strings.TrimSpace(dataRef)
				if ref == "" {
					return "", common.ValidationErrorf("isv-block type data-ref cannot be empty; expected reference_map.isv-block.<ref>").WithParam("data-ref")
				}
				if _, ok := refMap[isvBlockTag][ref]; !ok {
					return "", common.ValidationErrorf("reference_map.%s.%s is required for isv-block type data-ref", isvBlockTag, ref).WithParam("reference_map")
				}
				return tag.renderTag(isvBlockTag, false), nil
			}
			if !hasPath {
				return "", common.ValidationErrorf("isv-block type requires path=\"@relative.data\" or data-ref with reference_map.isv-block.<ref>").WithParam("isv-block")
			}

			data, err := readISVBlockPath(runtime, pathValue, "isv-block type path")
			if err != nil {
				return "", err
			}
			ref := nextRef()
			refMap[isvBlockTag][ref] = html5BlockReferenceEntry{Data: data}
			tag.removeAttrs(html5BlockPathAttr, html5BlockDataRefAttr, html5BlockDataAttr)
			tag.Attrs = append(tag.Attrs, html5BlockAttr{Name: html5BlockDataRefAttr, Value: ref})
			return tag.renderTag(isvBlockTag, false), nil
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

func validateISVBlockWriteElementBodies(format string, content string) error {
	validateSegment := func(segment string) error {
		matches := isvBlockElementPattern.FindAllStringSubmatchIndex(segment, -1)
		for _, match := range matches {
			if len(match) < 4 || match[2] < 0 || match[3] < 0 {
				continue
			}
			if strings.TrimSpace(segment[match[2]:match[3]]) != "" {
				return common.ValidationErrorf("isv-block type content must be loaded from path=\"@relative.data\" or reference_map.isv-block.<ref>; remove content between <isv-block> and </isv-block>").WithParam("isv-block")
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

func processISVBlockReferenceMapForFetch(runtime *common.RuntimeContext, format string, docToken string, data map[string]interface{}) error {
	doc, _ := data["document"].(map[string]interface{})
	if doc == nil {
		return nil
	}
	content, _ := doc["content"].(string)
	if !hasProcessableISVBlock(format, content) {
		return nil
	}

	refMap, err := referenceMapFromDocumentForBlock(doc, isvBlockTag)
	if err != nil {
		return err
	}
	group := refMap[isvBlockTag]
	if group == nil {
		return common.ValidationErrorf("document.reference_map.%s is required for fetched isv-block type content", isvBlockTag).WithParam("reference_map")
	}

	if err := validateFetchedISVBlockRefs(format, content, refMap); err != nil {
		return err
	}

	changed := false
	for ref, entry := range group {
		if entry.Data == "" || len([]byte(entry.Data)) <= html5BlockReferenceMaxRaw {
			continue
		}
		relPath, err := writeISVBlockReferenceFile(runtime, docToken, ref, entry.Data)
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

func referenceMapFromDocumentForBlock(doc map[string]interface{}, blockTag string) (html5BlockReferenceMap, error) {
	raw, ok := doc["reference_map"]
	if !ok || raw == nil {
		return nil, common.ValidationErrorf("document.reference_map.%s is required for fetched %s content", blockTag, blockTag).WithParam("reference_map")
	}
	refMap, err := referenceMapFromValue(raw, "document.reference_map")
	if err != nil {
		return nil, err
	}
	if len(refMap) == 0 {
		return nil, common.ValidationErrorf("document.reference_map.%s is required for fetched %s content", blockTag, blockTag).WithParam("reference_map")
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

func validateFetchedISVBlockRefs(format string, content string, refMap html5BlockReferenceMap) error {
	validateSegment := func(segment string) error {
		_, err := rewriteISVBlockStartTags(segment, func(raw string) (string, error) {
			tag, parseErr := parseISVBlockStartTag(raw)
			if parseErr != nil {
				return raw, common.ValidationErrorf("invalid isv-block type tag in fetched content for reference_map.isv-block.<ref>: %v", parseErr).WithParam("isv-block")
			}
			if !tag.hasAttr(isvBlockTypeAttr) {
				return raw, common.ValidationErrorf("fetched isv-block is missing type; cannot resolve reference_map.isv-block.<ref>").WithParam("type")
			}
			ref, ok := tag.attr(html5BlockDataRefAttr)
			if !ok || strings.TrimSpace(ref) == "" {
				return raw, common.ValidationErrorf("fetched isv-block type is missing data-ref; cannot resolve reference_map.isv-block.<ref>").WithParam("isv-block")
			}
			ref = strings.TrimSpace(ref)
			if _, ok := refMap[isvBlockTag][ref]; !ok {
				return raw, common.ValidationErrorf("document.reference_map.%s.%s is missing; cannot resolve isv-block type. Re-run fetch or check that the upstream document.reference_map field includes this ref.", isvBlockTag, ref).WithParam("reference_map")
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

func resolveISVReferenceMapPaths(runtime *common.RuntimeContext, refMap html5BlockReferenceMap) error {
	group := refMap[isvBlockTag]
	for ref, entry := range group {
		if strings.TrimSpace(entry.Path) == "" {
			continue
		}
		if entry.Data != "" {
			return common.ValidationErrorf("reference_map.%s.%s must use either data or path, not both, for isv-block type", isvBlockTag, ref).WithParam("reference_map")
		}
		data, err := readISVBlockPath(runtime, entry.Path, fmt.Sprintf("reference_map.%s.%s.path for isv-block type", isvBlockTag, ref))
		if err != nil {
			return err
		}
		entry.Data = data
		entry.Path = ""
		group[ref] = entry
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

func readISVBlockPath(runtime *common.RuntimeContext, pathValue string, label string) (string, error) {
	pathRaw := strings.TrimSpace(pathValue)
	if !strings.HasPrefix(pathRaw, "@") {
		return "", common.ValidationErrorf("%s %q must start with @, for example @payload.data; use reference_map.isv-block.<ref>.data for inline data", label, pathValue).WithParam("path")
	}
	relPath := strings.TrimSpace(strings.TrimPrefix(pathRaw, "@"))
	if relPath == "" {
		return "", common.ValidationErrorf("%s cannot be empty after @; expected reference_map.isv-block.<ref>.data or path=\"@relative.data\"", label).WithParam("path")
	}
	clean := filepath.Clean(relPath)
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", common.ValidationErrorf("%s %q must be a relative path within the current working directory for reference_map.isv-block.<ref>", label, pathValue).WithParam("path")
	}
	if strings.ToLower(filepath.Ext(clean)) != ".data" {
		return "", common.ValidationErrorf("%s %q must point to a .data file for reference_map.isv-block.<ref>", label, pathValue).WithParam("path")
	}
	data, err := cmdutil.ReadInputFile(runtime.FileIO(), clean)
	if err != nil {
		return "", common.ValidationErrorf("%s %q cannot be read from the current working directory; check that the file exists relative to where lark-cli is running and matches reference_map.isv-block.<ref>: %v", label, clean, err).WithParam("path").WithCause(err)
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

func hasProcessableISVBlock(format string, content string) bool {
	if !strings.Contains(content, "<isv-block") {
		return false
	}
	if strings.TrimSpace(format) != "markdown" {
		return true
	}
	found := false
	_ = applyOutsideCodeFences(content, func(segment string) string {
		if strings.Contains(segment, "<isv-block") {
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

func isvReferenceMapFromObject(refMap map[string]interface{}) (html5BlockReferenceMap, error) {
	if len(refMap) == 0 {
		return nil, nil
	}
	group, ok := refMap[isvBlockTag]
	if !ok || group == nil {
		return nil, nil
	}
	return referenceMapFromValue(map[string]interface{}{isvBlockTag: group}, "reference_map."+isvBlockTag)
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

func mergeISVReferenceMap(refMap map[string]interface{}, isvRefMap html5BlockReferenceMap) map[string]interface{} {
	group := isvRefMap[isvBlockTag]
	if len(group) == 0 {
		return refMap
	}
	if refMap == nil {
		refMap = map[string]interface{}{}
	}
	refMap[isvBlockTag] = group
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

func nextISVBlockRef(refMap html5BlockReferenceMap) func() string {
	next := 1
	return func() string {
		for {
			ref := fmt.Sprintf("%s_%d", isvBlockRefPrefix, next)
			next++
			if _, exists := refMap[isvBlockTag][ref]; !exists {
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

func writeISVBlockReferenceFile(runtime *common.RuntimeContext, docToken string, ref string, data string) (string, error) {
	if !isSafeHTML5BlockResourceName(docToken) {
		return "", common.ValidationErrorf("document_id %q cannot be used as a resource directory name for isv-block type reference_map.isv-block.<ref>", docToken).WithParam("document_id")
	}
	if !isSafeHTML5BlockResourceName(ref) {
		return "", common.ValidationErrorf("isv-block type data-ref %q cannot be used as a file name for reference_map.isv-block.<ref>", ref).WithParam("data-ref")
	}
	relPath := filepath.Join(html5BlockReferenceRoot, docToken, ref+".data")
	raw := []byte(data)
	_, err := runtime.FileIO().Save(relPath, fileio.SaveOptions{
		ContentType:   "application/octet-stream",
		ContentLength: int64(len(raw)),
	}, bytes.NewReader(raw))
	if err != nil {
		if errors.Is(err, fileio.ErrPathValidation) {
			return "", common.ValidationErrorf("cannot write isv-block type reference_map.isv-block.<ref> file %q: %v", relPath, err).WithParam("reference_map").WithCause(err)
		}
		return "", errs.NewInternalError(errs.SubtypeFileIO, "cannot write isv-block reference file %q: %v", relPath, err).WithCause(err)
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

func rewriteISVBlockStartTags(content string, fn func(raw string) (string, error)) (string, error) {
	var rewriteErr error
	out := isvBlockStartTagPattern.ReplaceAllStringFunc(content, func(raw string) string {
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
	return parseBlockStartTag(raw, html5BlockTag)
}

func parseISVBlockStartTag(raw string) (html5BlockStartTag, error) {
	return parseBlockStartTag(raw, isvBlockTag)
}

func parseBlockStartTag(raw string, expectedTag string) (html5BlockStartTag, error) {
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
		if start.Name.Local != expectedTag {
			return html5BlockStartTag{}, fmt.Errorf("expected <%s>, got <%s>", expectedTag, start.Name.Local) //nolint:forbidigo // intermediate parse helper; callers wrap with typed validation errors.
		}
		attrs := make([]html5BlockAttr, 0, len(start.Attr))
		for _, attr := range start.Attr {
			attrs = append(attrs, html5BlockAttr{Name: attr.Name.Local, Value: attr.Value})
		}
		return html5BlockStartTag{Attrs: attrs, SelfClosing: selfClosing}, nil
	}
	return html5BlockStartTag{}, fmt.Errorf("missing start element") //nolint:forbidigo // intermediate parse helper; callers wrap with typed validation errors.
}

func parseWhiteboardStartTag(raw string) (whiteboardStartTag, error) {
	trimmed := strings.TrimSpace(raw)
	selfClosing := strings.HasSuffix(trimmed, "/>")
	decoder := xml.NewDecoder(strings.NewReader(raw))
	for {
		tok, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return whiteboardStartTag{}, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != whiteboardTag {
			return whiteboardStartTag{}, fmt.Errorf("expected <%s>, got <%s>", whiteboardTag, start.Name.Local) //nolint:forbidigo // intermediate parse helper; callers wrap with typed validation errors.
		}
		attrs := make([]html5BlockAttr, 0, len(start.Attr))
		for _, attr := range start.Attr {
			attrs = append(attrs, html5BlockAttr{Name: attr.Name.Local, Value: attr.Value})
		}
		return whiteboardStartTag{Attrs: attrs, SelfClosing: selfClosing}, nil
	}
	return whiteboardStartTag{}, fmt.Errorf("missing start element") //nolint:forbidigo // intermediate parse helper; callers wrap with typed validation errors.
}

func (t html5BlockStartTag) attr(name string) (string, bool) {
	for _, attr := range t.Attrs {
		if attr.Name == name {
			return attr.Value, true
		}
	}
	return "", false
}

func (t whiteboardStartTag) attr(name string) (string, bool) {
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

func (t *whiteboardStartTag) removeAttrs(names ...string) {
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

func (t *whiteboardStartTag) setAttr(name string, value string) {
	for i, attr := range t.Attrs {
		if attr.Name == name {
			t.Attrs[i].Value = value
			return
		}
	}
	t.Attrs = append(t.Attrs, html5BlockAttr{Name: name, Value: value})
}

func (t html5BlockStartTag) render(selfClosing bool) string {
	return t.renderTag(html5BlockTag, selfClosing)
}

func (t html5BlockStartTag) renderTag(tag string, selfClosing bool) string {
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(tag)
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
		b.WriteString(tag)
		b.WriteByte('>')
	}
	return b.String()
}

func (t whiteboardStartTag) render(selfClosing bool) string {
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(whiteboardTag)
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

func escapeXMLText(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
