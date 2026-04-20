// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/width"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// apiCaller is the subset of *common.RuntimeContext used by the table-width
// orchestration. Exposing it as a function type keeps this package testable
// without a fake for the whole runtime surface.
type apiCaller func(method, url string, params map[string]interface{}, data interface{}) (map[string]interface{}, error)

// widthRatioTotal is the denominator used by Feishu's
// update_grid_column_width_ratio API: width_ratios must sum to 100.
const widthRatioTotal = 100

// docxBlockTypeTable is the block_type value returned by
// /open-apis/docx/v1/documents/{id}/blocks for a docx table block.
const docxBlockTypeTable = 31

// extractMarkdownTables parses GFM pipe tables from markdown text and returns
// each table as a slice of rows, where each row is a slice of cell strings.
// Only sequences that follow the GFM shape — a header row immediately followed
// by a separator row whose cell count matches the header — are recognised as
// tables. This matches the Lark renderer's behaviour and avoids false
// positives on stray prose that happens to contain pipes.
//
// Body rows are normalised to the confirmed column count so extra or missing
// cells do not produce a ratio list that disagrees with the rendered table's
// column_size (which would otherwise cause the subsequent PATCH to be skipped).
//
// Fenced code blocks are skipped. Fence tracking honours the CommonMark rule
// that a fence only closes on a matching fence character (backtick vs tilde)
// of equal-or-longer length; shorter fence markers inside a longer fence do
// not terminate it.
func extractMarkdownTables(md string) [][][]string {
	var tables [][][]string
	// Parser states:
	//   nil header, nil current  → scanning prose
	//   non-nil header, nil current → saw candidate header, awaiting separator
	//   nil header, non-nil current → inside a confirmed table, accumulating rows
	var header []string
	var current [][]string
	tableCols := 0

	var fenceChar byte // 0 when not in a fence; '`' or '~' while inside
	var fenceLen int

	flushCurrent := func() {
		if current != nil {
			tables = append(tables, current)
			current = nil
			tableCols = 0
		}
	}

	normaliseRow := func(row []string, cols int) []string {
		switch {
		case len(row) > cols:
			return row[:cols]
		case len(row) < cols:
			padded := make([]string, cols)
			copy(padded, row)
			return padded
		}
		return row
	}

	for _, raw := range strings.Split(md, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)

		if c, n, ok := detectFenceMarker(trimmed); ok {
			switch {
			case fenceChar == 0:
				// Entering a fence; remember the marker.
				flushCurrent()
				header = nil
				fenceChar = c
				fenceLen = n
			case c == fenceChar && n >= fenceLen:
				// Closing the active fence with a matching marker of
				// equal-or-longer length.
				fenceChar = 0
				fenceLen = 0
			}
			// Any other fence marker inside an open fence is ignored.
			continue
		}
		if fenceChar != 0 {
			continue
		}

		if !isPipeTableRow(trimmed) {
			// Any non-pipe line flushes the current table and discards a
			// pending header candidate (the header was never confirmed).
			flushCurrent()
			header = nil
			continue
		}

		// Line is a pipe row. Route by parser state.
		switch {
		case header != nil:
			// Awaiting a separator with the same cell count as the header.
			if isPipeSeparatorRow(trimmed) {
				sep := splitPipeRow(trimmed)
				if len(sep) == len(header) {
					tableCols = len(header)
					current = [][]string{header}
					header = nil
					continue
				}
				// Separator cell count disagrees with the header; not a
				// valid GFM table. Drop the pending header entirely
				// rather than treat the separator as a new candidate.
				header = nil
				continue
			}
			// Two consecutive data rows without a separator don't form a
			// GFM table; drop the pending header but keep the current
			// line as a fresh candidate for a table that may follow.
			header = splitPipeRow(trimmed)
		case current != nil:
			// Inside a confirmed table.
			if isPipeSeparatorRow(trimmed) {
				// Separator rows after the confirmed one are unusual; drop.
				continue
			}
			current = append(current, normaliseRow(splitPipeRow(trimmed), tableCols))
		default:
			// Scanning prose and saw a pipe row: candidate header.
			if isPipeSeparatorRow(trimmed) {
				// A separator row with no preceding header is not a table.
				continue
			}
			header = splitPipeRow(trimmed)
		}
	}
	flushCurrent()
	return tables
}

// detectFenceMarker recognises a CommonMark-style code fence opening or
// closing marker. Returns the fence character (backtick or tilde), the run
// length, and true when the trimmed line is a valid fence marker line — that
// is, at least three identical fence chars optionally followed by an info
// string. Text after the opening run is treated as an info string and does
// not affect fence closure.
func detectFenceMarker(trimmed string) (byte, int, bool) {
	if len(trimmed) < 3 {
		return 0, 0, false
	}
	c := trimmed[0]
	if c != '`' && c != '~' {
		return 0, 0, false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == c {
		n++
	}
	if n < 3 {
		return 0, 0, false
	}
	// Backtick fences disallow backticks in the info string per CommonMark;
	// a tilde fence accepts any info string. For our purposes we only need
	// to know the char + length to decide open/close.
	return c, n, true
}

func isPipeTableRow(line string) bool {
	return strings.HasPrefix(line, "|") && strings.HasSuffix(line, "|") && len(line) >= 2
}

var pipeSeparatorCellRE = regexp.MustCompile(`^\s*:?-{3,}:?\s*$`)

func isPipeSeparatorRow(line string) bool {
	if !isPipeTableRow(line) {
		return false
	}
	for _, cell := range splitPipeRow(line) {
		if !pipeSeparatorCellRE.MatchString(cell) {
			return false
		}
	}
	return true
}

// splitPipeRow splits a pipe table row into cells, handling escaped \| inside
// cells by substituting a placeholder before splitting.
func splitPipeRow(line string) []string {
	inner := strings.TrimPrefix(strings.TrimSuffix(line, "|"), "|")
	// Protect escaped pipes.
	const placeholder = "\x00PIPE\x00"
	inner = strings.ReplaceAll(inner, `\|`, placeholder)
	parts := strings.Split(inner, "|")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.ReplaceAll(strings.TrimSpace(p), placeholder, "|")
	}
	return out
}

// computeWidthRatios returns column-width ratios that sum to widthRatioTotal.
// Each column's weight is the maximum visual width across its cells (first row
// included, header separator already removed). Empty tables or single-column
// tables return nil to signal "nothing to patch".
func computeWidthRatios(rows [][]string) []int {
	if len(rows) == 0 {
		return nil
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	if cols < 2 {
		return nil
	}
	// Every column must receive at least 1 in the final ratio (the API
	// rejects 0), so we cannot honour both the "≥1 per column" floor and
	// the "sum == widthRatioTotal" contract when the column count exceeds
	// widthRatioTotal. Give up and let the server keep its equal-width
	// default rather than emit an invalid width_ratios array.
	if cols > widthRatioTotal {
		return nil
	}

	weights := make([]int, cols)
	for _, r := range rows {
		for i := 0; i < cols; i++ {
			if i >= len(r) {
				continue
			}
			w := visualWidth(r[i])
			if w > weights[i] {
				weights[i] = w
			}
		}
	}

	// Ensure every column has at least weight 1 so zero-width columns still
	// receive a non-zero ratio.
	total := 0
	for i, w := range weights {
		if w <= 0 {
			weights[i] = 1
			w = 1
		}
		total += w
	}
	if total <= 0 {
		return nil
	}

	ratios := make([]int, cols)
	allocated := 0
	// Integer apportionment with rounding; remaining error goes to the widest
	// column so the array sums to exactly widthRatioTotal.
	for i, w := range weights {
		ratios[i] = w * widthRatioTotal / total
		if ratios[i] < 1 {
			ratios[i] = 1
		}
		allocated += ratios[i]
	}
	if diff := widthRatioTotal - allocated; diff != 0 {
		widest := 0
		for i := 1; i < cols; i++ {
			if weights[i] > weights[widest] {
				widest = i
			}
		}
		// If piling the full error onto the widest column would push its
		// ratio below 1, the table cannot be expressed as a valid
		// width_ratios array that still sums to widthRatioTotal without
		// silently violating the ≥1 floor on another column. Give up and
		// return nil rather than write a bad array the API will reject.
		if ratios[widest]+diff < 1 {
			return nil
		}
		ratios[widest] += diff
	}
	return ratios
}

// visualWidth estimates the display width of s by counting each rune as 2 when
// it is East Asian Wide/Full (CJK, full-width punctuation) and 1 otherwise.
// Zero-width runes (control chars, format chars like ZWJ, non-spacing/enclosing
// marks such as combining accents and emoji variation selectors) contribute 0.
//
// Known over-counting: Regional Indicator pairs (flags like 🇺🇸) and emoji
// ZWJ sequences (👨‍👩‍👧‍👦) render as a single grapheme cluster of visual
// width 2 but this function counts each base codepoint independently, so a
// flag weighs 4 and a four-person family weighs 8. The weights are only used
// as proportional inputs to column-width ratios, so the practical impact is
// that cells dominated by flags or family emoji claim slightly more width
// than the eye expects. Accepting this until a grapheme-cluster segmenter
// (e.g. rivo/uniseg) becomes worth the dependency — see the visualWidth
// test cases that pin the current behaviour.
func visualWidth(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case r == 0 || r == '\t':
			// Tabs and NULs do not reliably contribute visual width.
			continue
		case unicode.IsControl(r),
			unicode.In(r, unicode.Cf, unicode.Mn, unicode.Me):
			// Format (Cf: ZWJ, LRM, ...), non-spacing marks (Mn: combining
			// accents, emoji variation selectors U+FE0F), and enclosing marks
			// (Me) do not advance the cursor.
			continue
		case isWideRune(r):
			w += 2
		default:
			w++
		}
	}
	return w
}

// isWideRune returns true for runes that occupy two columns in a monospace
// grid. Classification is delegated to golang.org/x/text/width for the Unicode
// East Asian Width property (covers CJK, Hangul, full-width forms, etc.), with
// explicit ranges added for emoji and common symbol blocks that Unicode marks
// as "neutral" but most terminals and the Lark renderer show at 2 columns.
func isWideRune(r rune) bool {
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return true
	}
	switch {
	case r >= 0x2600 && r <= 0x26FF: // Misc Symbols (☀ ☁ ⚡ ♻ …)
		return true
	case r >= 0x2700 && r <= 0x27BF: // Dingbats (✅ ✈ ✂ ✏ …)
		return true
	case r >= 0x1F000 && r <= 0x1F2FF: // Mahjong / Playing Cards / Enclosed Alphanumeric (🀄 🃏 🅰 🆗 …)
		return true
	case r >= 0x1F300 && r <= 0x1FAFF: // Emoji & pictographs
		return true
	case r >= 0x1F1E6 && r <= 0x1F1FF: // Regional Indicator Symbols (flag halves)
		return true
	}
	return false
}

// applyMarkdownTableColumnWidths calculates per-column width ratios for each
// pipe table in the supplied markdown and PATCHes matching table blocks in the
// given document with update_grid_column_width_ratio. Failures are logged and
// treated as non-fatal because the main content has already been created.
//
// Only tables whose local column count equals the remote table's column_size
// are patched; mismatches are skipped. Tables are matched to remote table
// blocks by document-order index.
func applyMarkdownTableColumnWidths(runtime *common.RuntimeContext, documentID, markdown string) {
	applyTableColumnWidths(runtime.CallAPI, runtime.IO().ErrOut, documentID, markdown)
}

// applyTableColumnWidths is the testable core. Callers hand in an apiCaller
// and an io.Writer so tests can exercise the orchestration (remote/local
// count mismatch, per-table column-count skip, PATCH error non-fatal
// handling) without touching the network.
func applyTableColumnWidths(api apiCaller, errOut io.Writer, documentID, markdown string) {
	if documentID == "" || strings.TrimSpace(markdown) == "" {
		return
	}
	tables := extractMarkdownTables(markdown)
	if len(tables) == 0 {
		return
	}

	remote, err := fetchTableBlocks(api, documentID)
	if err != nil {
		fmt.Fprintf(errOut, "column-width adjustment skipped: %v\n", err)
		return
	}
	// Strict precondition: index-based pairing is only safe when the local
	// markdown and the remote document expose the same number of tables.
	// Diverging counts can happen when the extractor misses a form the server
	// accepts (tables nested in blockquotes, non-conforming separator rows),
	// or when the target doc was not fully overwritten. In either case,
	// proceeding would silently shift every subsequent pair and write wrong
	// ratios to tables that happen to match on column count.
	if len(remote) != len(tables) {
		fmt.Fprintf(
			errOut,
			"column-width adjustment skipped: remote has %d table block(s) but local markdown has %d; skipping to avoid misaligned ratios\n",
			len(remote), len(tables),
		)
		return
	}
	patched := 0
	for i := range tables {
		ratios := computeWidthRatios(tables[i])
		if ratios == nil {
			continue
		}
		if remote[i].ColumnSize != len(ratios) {
			fmt.Fprintf(
				errOut,
				"column-width skipped for table %d (block %s): local has %d columns, remote has %d\n",
				i+1, remote[i].BlockID, len(ratios), remote[i].ColumnSize,
			)
			continue
		}
		body := map[string]interface{}{
			"update_grid_column_width_ratio": map[string]interface{}{
				"width_ratios": ratios,
			},
		}
		url := fmt.Sprintf(
			"/open-apis/docx/v1/documents/%s/blocks/%s",
			validate.EncodePathSegment(documentID),
			validate.EncodePathSegment(remote[i].BlockID),
		)
		if _, err := api("PATCH", url, nil, body); err != nil {
			fmt.Fprintf(
				errOut,
				"column-width PATCH failed for block %s: %v\n",
				remote[i].BlockID, err,
			)
			continue
		}
		patched++
	}
	if patched > 0 {
		fmt.Fprintf(errOut, "column widths applied to %d/%d tables\n", patched, len(tables))
	}
}

// docxTokenForUpdate returns the docx token for the supplied --doc input.
// Docx URLs / bare tokens short-circuit; wiki inputs are resolved to their
// backing docx via /open-apis/wiki/v2/spaces/get_node so that column-width
// application works uniformly regardless of how the user referenced the doc.
func docxTokenForUpdate(runtime *common.RuntimeContext, doc string) (string, bool) {
	return docxTokenForAutoWidths(runtime.CallAPI, doc)
}

// docxTokenForAutoWidths is the testable core: it normalises a user-supplied
// doc token or URL to a docx token, optionally following a single wiki→docx
// resolve call. Callers hand in an apiCaller so tests can stub the network.
func docxTokenForAutoWidths(api apiCaller, doc string) (string, bool) {
	ref, err := parseDocumentRef(doc)
	if err != nil {
		return "", false
	}
	switch ref.Kind {
	case "docx":
		return ref.Token, true
	case "wiki":
		return resolveWikiNodeToDocx(api, ref.Token)
	}
	return "", false
}

// resolveWikiNodeToDocx maps a wiki node token to its backing docx object
// token using the public wiki API. Returns false for non-docx-backed nodes
// (sheets, bitables, etc.) so the caller can skip cleanly.
func resolveWikiNodeToDocx(api apiCaller, wikiToken string) (string, bool) {
	resp, err := api(
		"GET",
		"/open-apis/wiki/v2/spaces/get_node",
		map[string]interface{}{"token": wikiToken},
		nil,
	)
	if err != nil || resp == nil {
		return "", false
	}
	node, _ := resp["node"].(map[string]interface{})
	if node == nil {
		return "", false
	}
	if objType, _ := node["obj_type"].(string); objType != "docx" {
		return "", false
	}
	token, _ := node["obj_token"].(string)
	return token, token != ""
}

// resolveDocxTokenForCreateResult extracts the docx token from an MCP
// create-doc result, preferring the doc_url (unambiguous about Kind) and
// falling back to doc_id. Wiki URLs surfaced in the response are resolved to
// docx via the public wiki API so the auto-widths path still runs.
func resolveDocxTokenForCreateResult(api apiCaller, result map[string]interface{}) (string, bool) {
	if docURL := strings.TrimSpace(common.GetString(result, "doc_url")); docURL != "" {
		if ref, err := parseDocumentRef(docURL); err == nil {
			switch ref.Kind {
			case "docx":
				return ref.Token, true
			case "wiki":
				return resolveWikiNodeToDocx(api, ref.Token)
			}
		}
	}
	if docID := strings.TrimSpace(common.GetString(result, "doc_id")); docID != "" {
		return docID, true
	}
	return "", false
}

// remoteTable describes the minimal fields we need about a docx table block.
type remoteTable struct {
	BlockID    string
	ColumnSize int
}

// fetchTableBlocks returns all table blocks (block_type == 31) in the given
// docx document, in document order. The function walks the paginated /blocks
// endpoint via the supplied apiCaller.
func fetchTableBlocks(api apiCaller, documentID string) ([]remoteTable, error) {
	var out []remoteTable
	pageToken := ""
	for {
		params := map[string]interface{}{"page_size": float64(500)}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		resp, err := api(
			"GET",
			fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks", validate.EncodePathSegment(documentID)),
			params,
			nil,
		)
		if err != nil {
			return nil, output.Errorf(output.ExitAPI, "api_error", "fetch blocks: %v", err)
		}
		items, _ := resp["items"].([]interface{})
		for _, item := range items {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			bt, _ := m["block_type"].(float64)
			if int(bt) != docxBlockTypeTable {
				continue
			}
			blockID, _ := m["block_id"].(string)
			table, _ := m["table"].(map[string]interface{})
			prop, _ := table["property"].(map[string]interface{})
			cs, _ := prop["column_size"].(float64)
			if blockID == "" || cs == 0 {
				continue
			}
			out = append(out, remoteTable{BlockID: blockID, ColumnSize: int(cs)})
		}
		hasMore, _ := resp["has_more"].(bool)
		next, _ := resp["page_token"].(string)
		if !hasMore || next == "" || next == pageToken {
			break
		}
		pageToken = next
	}
	return out, nil
}
