// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// Client-side emulation for block_move_after / block_copy_insert_after.
//
// Some server deployments do not read src_block_ids and reject both commands
// with "requires content or src_block_ids but both were empty"
// (https://github.com/larksuite/cli/issues/758). --emulate opts into a
// client-side fallback that reproduces move/copy with the primitives that do
// work:
//
//	fetch source blocks (full detail) → rebuild them after the anchor via
//	block_insert_after → re-fetch to verify the rebuilt blocks exist →
//	block_delete the originals (move only)
//
// The insert always happens before the delete and every failure stops the
// flow immediately with the inserted/deleted state reported on stderr, so no
// path loses the source content.
//
// Emulation is NOT semantically identical to the server-side commands:
//   - rebuilt blocks are assigned new block ids
//   - comments and #block_id anchor links on the originals are not migrated
//   - image blocks are re-uploaded from their source URL (file token changes)
//
// Block types that carry server-side tokens or state (whiteboard, sheet,
// bitable, table, synced_reference, ...) are rejected outright: a text-level
// rebuild would corrupt them, so refusing is the safe behavior.

var emulableBlockTagList = []string{
	"p", "h1", "h2", "h3", "h4", "h5", "h6", "h7", "h8", "h9",
	"ul", "ol", "li", "blockquote", "callout", "pre", "img",
}

var emulableBlockTags = func() map[string]bool {
	m := make(map[string]bool, len(emulableBlockTagList))
	for _, tag := range emulableBlockTagList {
		m[tag] = true
	}
	return m
}()

var (
	// emulateXMLTagRE tokenizes DocxXML tags: close marker, tag name,
	// attribute run (quote-aware so '>' inside attribute values is skipped),
	// self-close marker.
	emulateXMLTagRE = regexp.MustCompile(`<(/?)([a-zA-Z][\w:-]*)((?:"[^"]*"|'[^']*'|[^>"'])*?)(/?)>`)
	// emulateBlockIDRE extracts the block id attribute from an attribute run.
	emulateBlockIDRE = regexp.MustCompile(`(?:^|\s)id="([^"]+)"`)
	// emulateStripIDRE removes block id attributes before re-insert; the
	// server assigns fresh ids to rebuilt blocks.
	emulateStripIDRE = regexp.MustCompile(`\s+id="[^"]*"`)
	// emulateStripCalloutColorRE removes rgb(...) callout colors: fetch
	// returns them as rgb triples but the write path only accepts semantic
	// color names, so they are dropped (the emoji attribute is kept).
	emulateStripCalloutColorRE = regexp.MustCompile(`\s+(?:background-color|border-color|text-color)="rgb\([^"]*\)"`)
	// emulateImgTagRE matches an img tag (optionally with a separate close
	// tag) for href-based rebuilding.
	emulateImgTagRE = regexp.MustCompile(`<img\b((?:"[^"]*"|'[^']*'|[^>"'])*?)/?>(?:</img>)?`)
)

// emulatedBlock is one id-carrying block extracted from fetched DocxXML.
type emulatedBlock struct {
	id      string
	tag     string
	attrs   string
	outer   string   // full source slice including the tag itself
	parents []string // ancestor tag names, outermost first
	start   int      // byte offset in the fetched XML, used for document ordering
}

// indexEmulatedBlocks scans fetched DocxXML and indexes every block that
// carries an id attribute, keeping its full subtree slice and ancestor chain.
func indexEmulatedBlocks(xml string) map[string]emulatedBlock {
	byID := make(map[string]emulatedBlock)
	type openTag struct {
		name    string
		attrs   string
		start   int
		parents []string
	}
	var stack []openTag

	finish := func(el openTag, end int) {
		m := emulateBlockIDRE.FindStringSubmatch(el.attrs)
		if m == nil {
			return
		}
		byID[m[1]] = emulatedBlock{
			id:      m[1],
			tag:     el.name,
			attrs:   el.attrs,
			outer:   xml[el.start:end],
			parents: el.parents,
			start:   el.start,
		}
	}

	stackNames := func() []string {
		names := make([]string, len(stack))
		for i, el := range stack {
			names[i] = el.name
		}
		return names
	}

	for _, idx := range emulateXMLTagRE.FindAllStringSubmatchIndex(xml, -1) {
		group := func(n int) string {
			if idx[2*n] < 0 {
				return ""
			}
			return xml[idx[2*n]:idx[2*n+1]]
		}
		name := group(2)
		switch {
		case group(1) == "/": // close tag: pop to the nearest matching open tag
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].name == name {
					finish(stack[i], idx[1])
					stack = stack[:i] // unclosed children are dropped with the parent
					break
				}
			}
		case group(4) == "/": // self-closing tag
			finish(openTag{name: name, attrs: group(3), start: idx[0], parents: stackNames()}, idx[1])
		default:
			stack = append(stack, openTag{name: name, attrs: group(3), start: idx[0], parents: stackNames()})
		}
	}
	return byID
}

func xmlAttrValue(attrs, name string) (string, bool) {
	re := regexp.MustCompile(`(?:^|\s)` + regexp.QuoteMeta(name) + `="([^"]*)"`)
	m := re.FindStringSubmatch(attrs)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func xmlAttrEscape(raw string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(raw)
}

func xmlEntityDecode(s string) string {
	r := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&amp;", "&")
	return r.Replace(s)
}

// rebuildEmulatedImgTag rewrites an img block from its href: the fetched src
// token / mime / scale attributes cannot be written back, so re-inserting the
// image is effectively a re-upload from the source URL.
func rebuildEmulatedImgTag(attrs string) (string, error) {
	href, ok := xmlAttrValue(attrs, "href")
	if !ok {
		href, ok = xmlAttrValue(attrs, "url")
	}
	if !ok {
		return "", errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"cannot emulate move/copy of an img block without an href/url attribute; the fetched src token cannot be re-inserted").
			WithHint("re-insert the image manually, or use docs +media-insert with the original file")
	}
	var b strings.Builder
	b.WriteString(`<img href="`)
	b.WriteString(xmlAttrEscape(xmlEntityDecode(href)))
	b.WriteString(`"`)
	for _, name := range []string{"width", "height", "caption", "name"} {
		if v, ok := xmlAttrValue(attrs, name); ok && v != "" {
			fmt.Fprintf(&b, ` %s="%s"`, name, v)
		}
	}
	b.WriteString("/>")
	return b.String(), nil
}

// sanitizeEmulatedBlockXML turns a fetched block subtree into content the
// write path accepts: img tags are rebuilt from href, block ids are stripped
// (the server assigns new ones), and rgb callout colors are dropped.
func sanitizeEmulatedBlockXML(xml string) (string, error) {
	var rebuildErr error
	out := emulateImgTagRE.ReplaceAllStringFunc(xml, func(tag string) string {
		m := emulateImgTagRE.FindStringSubmatch(tag)
		rebuilt, err := rebuildEmulatedImgTag(m[1])
		if err != nil && rebuildErr == nil {
			rebuildErr = err
		}
		return rebuilt
	})
	if rebuildErr != nil {
		return "", rebuildErr
	}
	out = emulateStripIDRE.ReplaceAllString(out, "")
	out = emulateStripCalloutColorRE.ReplaceAllString(out, "")
	return out, nil
}

// buildEmulatedBlockContent produces the block_insert_after payload for one
// source block, rejecting block types a text-level rebuild would corrupt.
func buildEmulatedBlockContent(b emulatedBlock) (string, error) {
	if !emulableBlockTags[b.tag] {
		return "", errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"cannot emulate move/copy of block %s: <%s> blocks carry server-side tokens or state that a client-side rebuild would corrupt", b.id, b.tag).
			WithHint("supported block types: %s; move other block types in the editor instead", strings.Join(emulableBlockTagList, ", "))
	}
	inner, err := sanitizeEmulatedBlockXML(b.outer)
	if err != nil {
		return "", err
	}
	if b.tag == "li" {
		// A bare li is invalid insert content; wrap it in the nearest ancestor
		// list type so ordered items stay ordered.
		listTag := "ul"
		for i := len(b.parents) - 1; i >= 0; i-- {
			if b.parents[i] == "ul" || b.parents[i] == "ol" {
				listTag = b.parents[i]
				break
			}
		}
		return fmt.Sprintf("<%s>%s</%s>", listTag, inner, listTag), nil
	}
	return inner, nil
}

func splitSrcBlockIDs(raw string) []string {
	var ids []string
	for _, id := range strings.Split(raw, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// emulateFetchBody builds the docs_ai fetch request body for the emulation
// path. The first fetch needs full detail so img blocks carry a rebuildable
// href; the verification fetch only needs block ids.
func emulateFetchBody(runtime *common.RuntimeContext, full bool) map[string]interface{} {
	exportOption := map[string]interface{}{"export_block_id": true}
	if full {
		exportOption["export_style_attrs"] = true
		exportOption["export_cite_extra_data"] = true
	}
	body := map[string]interface{}{
		"format":        "xml",
		"export_option": exportOption,
	}
	injectDocsScene(runtime, body)
	return body
}

// emulateFetchDocumentXML fetches the document and returns its id and XML content.
func emulateFetchDocumentXML(runtime *common.RuntimeContext, token string, full bool) (docID, content string, err error) {
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", token)
	data, err := doDocAPI(runtime, "POST", apiPath, emulateFetchBody(runtime, full))
	if err != nil {
		return "", "", err
	}
	doc, _ := data["document"].(map[string]interface{})
	if doc == nil {
		return "", "", errs.NewInternalError(errs.SubtypeInvalidResponse, "fetch response has no document object")
	}
	docID, _ = doc["document_id"].(string)
	content, _ = doc["content"].(string)
	return docID, content, nil
}

// emulateUpdateBody builds a docs_ai update request body for one emulation write.
func emulateUpdateBody(runtime *common.RuntimeContext, command string, fields map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{
		"format":  "xml",
		"command": command,
	}
	for k, v := range fields {
		body[k] = v
	}
	injectDocsScene(runtime, body)
	return body
}

// emulateUpdateDoc issues one write and asserts the service-level result field,
// which can report a non-success outcome even when the API code is 0.
func emulateUpdateDoc(runtime *common.RuntimeContext, token, command string, fields map[string]interface{}) error {
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s", token)
	data, err := doDocAPI(runtime, "PUT", apiPath, emulateUpdateBody(runtime, command, fields))
	if err != nil {
		return err
	}
	if result, _ := data["result"].(string); result != "success" {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "%s returned result=%q, expected \"success\"", command, result)
	}
	return nil
}

var emulateSemanticDifferences = []string{
	"rebuilt blocks were assigned new block ids; the original ids are gone",
	"comments and #block_id anchor links on the original blocks are not migrated",
	"image blocks were re-uploaded from their source URL; file tokens changed",
}

// executeUpdateEmulated runs the client-side move/copy emulation. Write order
// is insert → verify → delete so a failure at any step leaves the source
// content in the document.
func executeUpdateEmulated(runtime *common.RuntimeContext, ref documentRef, command string) error {
	isMove := command == "block_move_after"
	anchor := runtime.Str("block-id")
	srcIDs := splitSrcBlockIDs(runtime.Str("src-block-ids"))
	srcList := strings.Join(srcIDs, ",")
	errOut := runtime.IO().ErrOut

	fmt.Fprintf(errOut, "Emulating %s client-side (--emulate; server-side src_block_ids workaround)\n", command)
	fmt.Fprintf(errOut, "warning: emulation differs from the server-side command: rebuilt blocks get new block ids; comments and #block_id anchors on the originals are not migrated; image blocks are re-uploaded\n")

	// Step 1: fetch with full detail — img blocks only carry a rebuildable
	// href at this level.
	fmt.Fprintf(errOut, "[1/4] Fetching document blocks (full detail)\n")
	docID, beforeXML, err := emulateFetchDocumentXML(runtime, ref.Token, true)
	if err != nil {
		return err
	}
	blocks := indexEmulatedBlocks(beforeXML)

	// Step 2: validate sources and build the rebuilt payload. Every check
	// happens before the first write so a rejection leaves the document
	// untouched.
	pieces := make([]string, 0, len(srcIDs))
	for _, id := range srcIDs {
		b, ok := blocks[id]
		if !ok {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "source block %s not found in document %s", id, docID).WithParam("--src-block-ids")
		}
		piece, err := buildEmulatedBlockContent(b)
		if err != nil {
			return err
		}
		pieces = append(pieces, piece)
	}
	if _, ok := blocks[anchor]; !ok && anchor != "-1" && anchor != docID {
		fmt.Fprintf(errOut, "note: anchor block %s is not in the fetched block index (normal when it is the page id); the server will validate it\n", anchor)
	}
	content := strings.Join(pieces, "")
	fmt.Fprintf(errOut, "[2/4] Rebuilt %d source block(s) (%d characters)\n", len(srcIDs), len(content))

	// Step 3: insert the rebuilt blocks at the anchor. The originals are not
	// touched until this insert is verified.
	fmt.Fprintf(errOut, "[3/4] Inserting rebuilt blocks after %s\n", anchor)
	if err := emulateUpdateDoc(runtime, ref.Token, "block_insert_after", map[string]interface{}{
		"block_id": anchor,
		"content":  content,
	}); err != nil {
		fmt.Fprintf(errOut, "insert failed; the document is unchanged and the original blocks %s are intact\n", srcList)
		return err
	}

	// Step 4: re-fetch and confirm new blocks actually appeared before
	// deleting anything.
	fmt.Fprintf(errOut, "[4/4] Verifying rebuilt blocks\n")
	_, afterXML, err := emulateFetchDocumentXML(runtime, ref.Token, false)
	if err != nil {
		fmt.Fprintf(errOut, "verification fetch failed AFTER the insert: the rebuilt blocks may now exist alongside the originals %s; inspect the document before retrying\n", srcList)
		return err
	}
	afterBlocks := indexEmulatedBlocks(afterXML)
	var newIDs []string
	for id, b := range afterBlocks {
		if _, existed := blocks[id]; !existed {
			newIDs = append(newIDs, b.id)
		}
	}
	sort.Slice(newIDs, func(i, j int) bool { return afterBlocks[newIDs[i]].start < afterBlocks[newIDs[j]].start })
	if len(newIDs) == 0 {
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"block_insert_after reported success but the re-fetch shows no new blocks; the original blocks %s were NOT deleted", srcList).
			WithHint("inspect the document manually before retrying")
	}

	srcDeleted := false
	if isMove {
		fmt.Fprintf(errOut, "Deleting original blocks %s\n", srcList)
		if err := emulateUpdateDoc(runtime, ref.Token, "block_delete", map[string]interface{}{
			"block_id": srcList,
		}); err != nil {
			fmt.Fprintf(errOut, "delete failed AFTER a verified insert: the document now contains BOTH the originals (%s) and the rebuilt blocks (%s); delete one set manually\n", srcList, strings.Join(newIDs, ","))
			return err
		}
		srcDeleted = true
	}

	runtime.OutRaw(map[string]interface{}{
		"result":               "success",
		"emulated":             true,
		"command":              command,
		"document_id":          docID,
		"anchor_block_id":      anchor,
		"src_block_ids":        srcIDs,
		"src_blocks_deleted":   srcDeleted,
		"new_block_ids":        newIDs,
		"semantic_differences": emulateSemanticDifferences,
	}, nil)
	return nil
}

// dryRunUpdateEmulated describes the multi-call emulation plan without
// touching the document. The insert content placeholder stands in for the
// rebuilt blocks, which only exist after the first fetch.
func dryRunUpdateEmulated(runtime *common.RuntimeContext, ref documentRef, command string) *common.DryRunAPI {
	fetchPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", ref.Token)
	updatePath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s", ref.Token)
	anchor := runtime.Str("block-id")
	srcBlockIDs := runtime.Str("src-block-ids")

	d := common.NewDryRunAPI().
		Desc(fmt.Sprintf("client-side %s emulation: fetch source blocks → rebuild after anchor → verify → delete originals (move only)", command)).
		POST(fetchPath).Desc("[1] fetch source blocks (full detail)").Body(emulateFetchBody(runtime, true)).
		PUT(updatePath).Desc("[2] insert rebuilt blocks after the anchor").Body(emulateUpdateBody(runtime, "block_insert_after", map[string]interface{}{
		"block_id": anchor,
		"content":  "<rebuilt source blocks>",
	})).
		POST(fetchPath).Desc("[3] verify the rebuilt blocks exist").Body(emulateFetchBody(runtime, false))
	if command == "block_move_after" {
		d.PUT(updatePath).Desc("[4] delete the original blocks").Body(emulateUpdateBody(runtime, "block_delete", map[string]interface{}{
			"block_id": srcBlockIDs,
		}))
	}
	return d.Set("document_id", ref.Token)
}
