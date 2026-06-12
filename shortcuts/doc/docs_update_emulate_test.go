// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

// emulateTestXML is a fetched-document fixture covering every rebuild case:
// heading, paragraph with entities, ordered-list item, image (href + src
// token), unsupported whiteboard, callout with rgb colors, and the anchor.
// Lark's DocxXML export is flat (top-level blocks are siblings with no
// wrapping <page>/<body> element), so the fixture mirrors that.
const emulateTestXML = `<h1 id="blkHead">Title</h1>` +
	`<p id="blkA">First &amp; second</p>` +
	`<ol id="blkList"><li id="blkLi">item</li></ol>` +
	`<img id="blkImg" src="tokimg" href="https://example.com/a.png?x=1&amp;y=2" width="100" height="50"/>` +
	`<whiteboard id="blkWb" token="wbtok"/>` +
	`<callout id="blkCall" background-color="rgb(1,2,3)" emoji="bulb"><p id="blkCallP">tip</p></callout>` +
	`<p id="blkAnchor">anchor</p>`

func TestIndexEmulatedBlocks(t *testing.T) {
	blocks := indexEmulatedBlocks(emulateTestXML)

	tests := []struct {
		id          string
		wantTag     string
		wantOuter   string
		wantParents []string
	}{
		{id: "blkA", wantTag: "p", wantOuter: `<p id="blkA">First &amp; second</p>`, wantParents: []string{}},
		{id: "blkLi", wantTag: "li", wantOuter: `<li id="blkLi">item</li>`, wantParents: []string{"ol"}},
		{id: "blkImg", wantTag: "img", wantParents: []string{}},
		{id: "blkWb", wantTag: "whiteboard", wantParents: []string{}},
		{id: "blkList", wantTag: "ol", wantOuter: `<ol id="blkList"><li id="blkLi">item</li></ol>`, wantParents: []string{}},
	}
	for _, tt := range tests {
		b, ok := blocks[tt.id]
		if !ok {
			t.Fatalf("block %s not indexed", tt.id)
		}
		if b.tag != tt.wantTag {
			t.Fatalf("block %s tag = %q, want %q", tt.id, b.tag, tt.wantTag)
		}
		if tt.wantOuter != "" && b.outer != tt.wantOuter {
			t.Fatalf("block %s outer = %q, want %q", tt.id, b.outer, tt.wantOuter)
		}
		if got := strings.Join(b.parents, ","); got != strings.Join(tt.wantParents, ",") {
			t.Fatalf("block %s parents = %q, want %q", tt.id, got, strings.Join(tt.wantParents, ","))
		}
	}
}

func TestBuildEmulatedBlockContent(t *testing.T) {
	blocks := indexEmulatedBlocks(emulateTestXML)

	tests := []struct {
		name            string
		id              string
		want            string
		wantInsertedTag string
	}{
		{name: "paragraph keeps entities and drops id", id: "blkA", want: `<p>First &amp; second</p>`, wantInsertedTag: "p"},
		{name: "list item wraps in nearest list type", id: "blkLi", want: `<ol><li>item</li></ol>`, wantInsertedTag: "ol"},
		{name: "image rebuilds from href and drops src token", id: "blkImg", want: `<img href="https://example.com/a.png?x=1&amp;y=2" width="100" height="50"/>`, wantInsertedTag: "img"},
		{name: "callout drops rgb colors keeps emoji", id: "blkCall", want: `<callout emoji="bulb"><p>tip</p></callout>`, wantInsertedTag: "callout"},
		{name: "list container keeps children", id: "blkList", want: `<ol><li>item</li></ol>`, wantInsertedTag: "ol"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, insertedTag, err := buildEmulatedBlockContent(blocks[tt.id])
			if err != nil {
				t.Fatalf("buildEmulatedBlockContent(%s) error: %v", tt.id, err)
			}
			if got != tt.want {
				t.Fatalf("buildEmulatedBlockContent(%s) content = %q, want %q", tt.id, got, tt.want)
			}
			if insertedTag != tt.wantInsertedTag {
				t.Fatalf("buildEmulatedBlockContent(%s) insertedTag = %q, want %q", tt.id, insertedTag, tt.wantInsertedTag)
			}
		})
	}
}

func TestBuildEmulatedBlockContentRejectsUnsupportedType(t *testing.T) {
	blocks := indexEmulatedBlocks(emulateTestXML)

	_, _, err := buildEmulatedBlockContent(blocks["blkWb"])
	// Typed validation contract: failed_precondition naming --src-block-ids so
	// the command layer knows which flag carried the offending source block.
	assertValidationContract(t, err, errs.SubtypeFailedPrecondition, "--src-block-ids")
	for _, want := range []string{"blkWb", "<whiteboard>"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

func TestBuildEmulatedBlockContentRejectsImgWithoutHref(t *testing.T) {
	// An img source block with no href/url cannot be re-inserted; the rejection
	// must surface as a typed validation fault naming --src-block-ids.
	blocks := indexEmulatedBlocks(`<img id="blkNoHref" src="tok123" width="10"/>`)

	_, _, err := buildEmulatedBlockContent(blocks["blkNoHref"])
	assertValidationContract(t, err, errs.SubtypeFailedPrecondition, "--src-block-ids")
	if !strings.Contains(err.Error(), "href") {
		t.Fatalf("error should mention the missing href: %v", err)
	}
}

func TestRebuildEmulatedImgTagRequiresHref(t *testing.T) {
	_, err := rebuildEmulatedImgTag(` src="tok123" width="10"`)
	if err == nil {
		t.Fatal("expected error for img without href/url")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %v", err)
	}
	if problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeFailedPrecondition)
	}
}

// docEmulateOutput decodes the emulation success envelope.
type docEmulateOutput struct {
	OK   bool `json:"ok"`
	Data struct {
		Result           string   `json:"result"`
		Emulated         bool     `json:"emulated"`
		Command          string   `json:"command"`
		DocumentID       string   `json:"document_id"`
		AnchorBlockID    string   `json:"anchor_block_id"`
		SrcBlockIDs      []string `json:"src_block_ids"`
		SrcBlocksDeleted bool     `json:"src_blocks_deleted"`
		NewBlockIDs      []string `json:"new_block_ids"`
	} `json:"data"`
}

func decodeDocEmulateOutput(t *testing.T, stdout *bytes.Buffer) docEmulateOutput {
	t.Helper()
	var out docEmulateOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode emulate output: %v; output=%s", err, stdout.String())
	}
	return out
}

func registerEmulateFetchStub(reg *httpmock.Registry, docID, content string) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/" + docID + "/fetch",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"document": map[string]interface{}{
					"document_id": docID,
					"content":     content,
				},
			},
		},
	}
	reg.Register(stub)
	return stub
}

// emulateUpdateRequestBody mirrors the docs_ai update request fields the
// emulation writes.
type emulateUpdateRequestBody struct {
	Command string `json:"command"`
	BlockID string `json:"block_id"`
	Content string `json:"content"`
}

func decodeEmulateUpdateBody(t *testing.T, stub *httpmock.Stub) emulateUpdateRequestBody {
	t.Helper()
	var body emulateUpdateRequestBody
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode captured update body: %v; body=%s", err, stub.CapturedBody)
	}
	return body
}

func registerEmulateUpdateStub(reg *httpmock.Registry, docID, command string) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/docs_ai/v1/documents/" + docID,
		BodyFilter: func(body []byte) bool {
			return strings.Contains(string(body), `"command":"`+command+`"`)
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"result": "success"},
		},
	}
	reg.Register(stub)
	return stub
}

func TestDocsUpdateEmulatedMoveViaExecute(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-emulate-move"))

	afterXML := strings.Replace(emulateTestXML,
		`<p id="blkAnchor">anchor</p>`,
		`<p id="blkAnchor">anchor</p><p id="blkNew">First &amp; second</p>`, 1)

	registerEmulateFetchStub(reg, "doxcnEmu", emulateTestXML)
	insertStub := registerEmulateUpdateStub(reg, "doxcnEmu", "block_insert_after")
	registerEmulateFetchStub(reg, "doxcnEmu", afterXML)
	deleteStub := registerEmulateUpdateStub(reg, "doxcnEmu", "block_delete")

	err := mountAndRunDocs(t, DocsUpdate, []string{
		"+update",
		"--doc", "doxcnEmu",
		"--command", "block_move_after",
		"--block-id", "blkAnchor",
		"--src-block-ids", "blkA",
		"--emulate",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	insertBody := decodeEmulateUpdateBody(t, insertStub)
	if insertBody.Content != `<p>First &amp; second</p>` {
		t.Fatalf("insert content = %q, want rebuilt paragraph without the source id", insertBody.Content)
	}
	if insertBody.BlockID != "blkAnchor" {
		t.Fatalf("insert block_id = %q, want %q", insertBody.BlockID, "blkAnchor")
	}
	if deleteBody := decodeEmulateUpdateBody(t, deleteStub); deleteBody.BlockID != "blkA" {
		t.Fatalf("delete block_id = %q, want %q", deleteBody.BlockID, "blkA")
	}

	out := decodeDocEmulateOutput(t, stdout)
	if !out.OK || out.Data.Result != "success" || !out.Data.Emulated {
		t.Fatalf("unexpected output envelope: %s", stdout.String())
	}
	if !out.Data.SrcBlocksDeleted {
		t.Fatal("move should delete the source blocks")
	}
	if len(out.Data.NewBlockIDs) != 1 || out.Data.NewBlockIDs[0] != "blkNew" {
		t.Fatalf("new_block_ids = %v, want [blkNew]", out.Data.NewBlockIDs)
	}
}

func TestDocsUpdateEmulatedCopyKeepsOriginals(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-emulate-copy"))

	afterXML := strings.Replace(emulateTestXML,
		`<p id="blkAnchor">anchor</p>`,
		`<p id="blkAnchor">anchor</p><ol id="blkNewList"><li id="blkNewLi">item</li></ol>`, 1)

	registerEmulateFetchStub(reg, "doxcnEmu", emulateTestXML)
	insertStub := registerEmulateUpdateStub(reg, "doxcnEmu", "block_insert_after")
	registerEmulateFetchStub(reg, "doxcnEmu", afterXML)
	// No block_delete stub: copy must not delete anything; Registry.Verify in
	// TestFactory cleanup fails the test on any unexpected call.

	err := mountAndRunDocs(t, DocsUpdate, []string{
		"+update",
		"--doc", "doxcnEmu",
		"--command", "block_copy_insert_after",
		"--block-id", "blkAnchor",
		"--src-block-ids", "blkLi",
		"--emulate",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// The rebuilt list item must be wrapped in its original ordered-list type.
	if insertBody := decodeEmulateUpdateBody(t, insertStub); insertBody.Content != "<ol><li>item</li></ol>" {
		t.Fatalf("insert content = %q, want wrapped list item", insertBody.Content)
	}

	out := decodeDocEmulateOutput(t, stdout)
	if !out.OK || !out.Data.Emulated || out.Data.Command != "block_copy_insert_after" {
		t.Fatalf("unexpected output envelope: %s", stdout.String())
	}
	if out.Data.SrcBlocksDeleted {
		t.Fatal("copy must not delete the source blocks")
	}
	if len(out.Data.NewBlockIDs) != 2 {
		t.Fatalf("new_block_ids = %v, want the two rebuilt list ids", out.Data.NewBlockIDs)
	}
}

func TestDocsUpdateEmulatedRejectsUnsupportedTypeBeforeWriting(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-emulate-reject"))

	// Only the initial fetch is stubbed: the unsupported block must be
	// rejected before any write request is issued.
	registerEmulateFetchStub(reg, "doxcnEmu", emulateTestXML)

	err := mountAndRunDocs(t, DocsUpdate, []string{
		"+update",
		"--doc", "doxcnEmu",
		"--command", "block_move_after",
		"--block-id", "blkAnchor",
		"--src-block-ids", "blkA,blkWb",
		"--emulate",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected unsupported-type error")
	}
	for _, want := range []string{"blkWb", "<whiteboard>"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

func TestDocsUpdateEmulatedStopsWhenInsertNotVisible(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-emulate-verify"))

	registerEmulateFetchStub(reg, "doxcnEmu", emulateTestXML)
	registerEmulateUpdateStub(reg, "doxcnEmu", "block_insert_after")
	// The verification fetch returns the unchanged document: no new block ids.
	registerEmulateFetchStub(reg, "doxcnEmu", emulateTestXML)
	// No block_delete stub: the originals must never be deleted on a failed
	// verification.

	err := mountAndRunDocs(t, DocsUpdate, []string{
		"+update",
		"--doc", "doxcnEmu",
		"--command", "block_move_after",
		"--block-id", "blkAnchor",
		"--src-block-ids", "blkA",
		"--emulate",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected verification error")
	}
	if !strings.Contains(err.Error(), "NOT deleted") {
		t.Fatalf("error should state the originals were kept: %v", err)
	}
}

func TestDocsUpdateEmulateValidation(t *testing.T) {
	tests := []struct {
		name     string
		setFlags map[string]string
		want     string
	}{
		{
			name:     "rejects non move/copy command",
			setFlags: map[string]string{"command": "append", "content": "<p>x</p>", "emulate": "true"},
			want:     "--emulate only applies to block_move_after and block_copy_insert_after",
		},
		{
			name: "rejects explicit revision id",
			setFlags: map[string]string{
				"command": "block_move_after", "block-id": "blkAnchor",
				"src-block-ids": "blkA", "emulate": "true", "revision-id": "7", "content": "",
			},
			want: "--revision-id is not supported with --emulate",
		},
		{
			name: "rejects duplicate source ids",
			setFlags: map[string]string{
				"command": "block_move_after", "block-id": "blkAnchor",
				"src-block-ids": "blkA,blkA", "emulate": "true", "content": "",
			},
			want: "duplicate block id blkA",
		},
		{
			name: "rejects empty source id list",
			setFlags: map[string]string{
				"command": "block_copy_insert_after", "block-id": "blkAnchor",
				"src-block-ids": ",", "emulate": "true",
			},
			want: "contains no block ids",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newUpdateShortcutTestRuntime(t, "", tt.setFlags)
			err := validateUpdateV2(context.Background(), runtime)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error missing %q: %v", tt.want, err)
			}
		})
	}
}

func TestDocsUpdateEmulatedDryRun(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		wantCalls int
	}{
		{name: "move plans fetch insert verify delete", command: "block_move_after", wantCalls: 4},
		{name: "copy plans fetch insert verify", command: "block_copy_insert_after", wantCalls: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newUpdateShortcutTestRuntime(t, "", map[string]string{
				"command": tt.command, "block-id": "blkAnchor",
				"src-block-ids": "blkA,blkB", "emulate": "true", "content": "",
			})
			if err := validateUpdateV2(context.Background(), runtime); err != nil {
				t.Fatalf("validateUpdateV2() error = %v", err)
			}

			dry := decodeDocDryRun(t, DocsUpdate.DryRun(context.Background(), runtime))
			if len(dry.API) != tt.wantCalls {
				t.Fatalf("expected %d dry-run API calls, got %d", tt.wantCalls, len(dry.API))
			}
			if got, want := dry.API[0].URL, "/open-apis/docs_ai/v1/documents/doxcnUpdateDryRun/fetch"; got != want {
				t.Fatalf("first dry-run URL = %q, want %q", got, want)
			}
			if got, want := dry.API[1].Body["command"], "block_insert_after"; got != want {
				t.Fatalf("insert command = %#v, want %q", got, want)
			}
			if tt.command == "block_move_after" {
				if got, want := dry.API[3].Body["command"], "block_delete"; got != want {
					t.Fatalf("delete command = %#v, want %q", got, want)
				}
				if got, want := dry.API[3].Body["block_id"], "blkA,blkB"; got != want {
					t.Fatalf("delete block_id = %#v, want %q", got, want)
				}
			}
		})
	}
}

func TestIndexTopLevelBlocks(t *testing.T) {
	got := indexTopLevelBlocks(emulateTestXML)

	type want struct {
		tag string
		ids []string
	}
	wants := []want{
		{tag: "h1", ids: []string{"blkHead"}},
		{tag: "p", ids: []string{"blkA"}},
		{tag: "ol", ids: []string{"blkLi"}}, // list container has no id; its li does
		{tag: "img", ids: []string{"blkImg"}},
		{tag: "whiteboard", ids: []string{"blkWb"}},
		{tag: "callout", ids: []string{"blkCall", "blkCallP"}}, // block + nested child
		{tag: "p", ids: []string{"blkAnchor"}},
	}
	if len(got) != len(wants) {
		t.Fatalf("top-level block count = %d, want %d (%+v)", len(got), len(wants), got)
	}
	for i, w := range wants {
		if got[i].tag != w.tag {
			t.Fatalf("block[%d] tag = %q, want %q", i, got[i].tag, w.tag)
		}
		for _, id := range w.ids {
			if !got[i].ids[id] {
				t.Fatalf("block[%d] (%s) missing id %q; ids=%v", i, w.tag, id, got[i].ids)
			}
		}
	}
}

func TestVerifyEmulatedInsert(t *testing.T) {
	beforeIDs := map[string]bool{"blkAnchor": true, "blkOld": true}
	// after: anchor, then a freshly inserted paragraph, then a pre-existing block.
	after := []topLevelBlock{
		{tag: "p", ids: map[string]bool{"blkAnchor": true}},
		{tag: "p", ids: map[string]bool{"blkNew": true}},
		{tag: "p", ids: map[string]bool{"blkOld": true}},
	}

	t.Run("matches contiguous run after anchor", func(t *testing.T) {
		newIDs, err := verifyEmulatedInsert(after, beforeIDs, "blkAnchor", "doxcnEmu", []string{"p"}, "blkSrc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(newIDs) != 1 || newIDs[0] != "blkNew" {
			t.Fatalf("newIDs = %v, want [blkNew]", newIDs)
		}
	})

	t.Run("rejects tag mismatch", func(t *testing.T) {
		_, err := verifyEmulatedInsert(after, beforeIDs, "blkAnchor", "doxcnEmu", []string{"h1"}, "blkSrc")
		if err == nil {
			t.Fatal("expected conservative failure on tag mismatch")
		}
	})

	t.Run("rejects when the next block is a pre-existing original", func(t *testing.T) {
		// A concurrent edit could leave a pre-existing block right after the
		// anchor; that is not our insert, so verification must fail.
		concurrent := []topLevelBlock{
			{tag: "p", ids: map[string]bool{"blkAnchor": true}},
			{tag: "p", ids: map[string]bool{"blkOld": true}},
		}
		_, err := verifyEmulatedInsert(concurrent, beforeIDs, "blkAnchor", "doxcnEmu", []string{"p"}, "blkSrc")
		if err == nil {
			t.Fatal("expected conservative failure: the block after the anchor is a pre-existing original")
		}
	})

	t.Run("rejects when anchor is missing", func(t *testing.T) {
		_, err := verifyEmulatedInsert(after, beforeIDs, "blkGone", "doxcnEmu", []string{"p"}, "blkSrc")
		if err == nil {
			t.Fatal("expected conservative failure when anchor cannot be located")
		}
	})

	t.Run("page id anchor inserts at document start", func(t *testing.T) {
		atStart := []topLevelBlock{
			{tag: "p", ids: map[string]bool{"blkNew": true}},
			{tag: "p", ids: map[string]bool{"blkAnchor": true}},
		}
		newIDs, err := verifyEmulatedInsert(atStart, beforeIDs, "doxcnEmu", "doxcnEmu", []string{"p"}, "blkSrc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(newIDs) != 1 || newIDs[0] != "blkNew" {
			t.Fatalf("newIDs = %v, want [blkNew]", newIDs)
		}
	})

	t.Run("-1 anchor matches the final inserted block at document end", func(t *testing.T) {
		atEnd := []topLevelBlock{
			{tag: "p", ids: map[string]bool{"blkOld": true}},
			{tag: "img", ids: map[string]bool{"blkNew": true}},
		}
		newIDs, err := verifyEmulatedInsert(atEnd, beforeIDs, "-1", "doxcnEmu", []string{"img"}, "blkSrc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(newIDs) != 1 || newIDs[0] != "blkNew" {
			t.Fatalf("newIDs = %v, want [blkNew]", newIDs)
		}
	})

	t.Run("-1 anchor rejects when the last block is a pre-existing original", func(t *testing.T) {
		// Our insert at -1 did not land last (e.g. concurrent edit appended);
		// the final block is a pre-existing original, so do not delete.
		atEnd := []topLevelBlock{
			{tag: "img", ids: map[string]bool{"blkNew": true}},
			{tag: "p", ids: map[string]bool{"blkOld": true}},
		}
		_, err := verifyEmulatedInsert(atEnd, beforeIDs, "-1", "doxcnEmu", []string{"img"}, "blkSrc")
		if err == nil {
			t.Fatal("expected conservative failure: the final block is a pre-existing original")
		}
	})
}

// TestDocsUpdateEmulatedRejectsConcurrentEditAtAnchor is the regression guard
// for the data-loss bug a naive "any new block id appeared" check invited: a
// collaborator adds an unrelated block elsewhere, but our insert did NOT land
// after the anchor. The old check would have seen *a* new id and deleted the
// originals; anchor-scoped verification must refuse and keep them.
func TestDocsUpdateEmulatedRejectsConcurrentEditAtAnchor(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-emulate-concurrent"))

	// Verification fetch: a brand-new block appeared, but at the document top —
	// nowhere near the anchor. (Simulates a concurrent collaborator edit while
	// our own insert is, say, lost or delayed.)
	afterXML := strings.Replace(emulateTestXML,
		`<h1 id="blkHead">Title</h1>`,
		`<p id="blkConcurrent">someone else typed this</p><h1 id="blkHead">Title</h1>`, 1)

	registerEmulateFetchStub(reg, "doxcnEmu", emulateTestXML)
	registerEmulateUpdateStub(reg, "doxcnEmu", "block_insert_after")
	registerEmulateFetchStub(reg, "doxcnEmu", afterXML)
	// No block_delete stub: the originals must survive.

	err := mountAndRunDocs(t, DocsUpdate, []string{
		"+update",
		"--doc", "doxcnEmu",
		"--command", "block_move_after",
		"--block-id", "blkAnchor",
		"--src-block-ids", "blkA",
		"--emulate",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected conservative verification failure, not a false success")
	}
	if !strings.Contains(err.Error(), "NOT deleted") {
		t.Fatalf("error should state the originals were kept: %v", err)
	}
}

// TestDocsUpdateEmulatedMoveSucceedsDespiteConcurrentEditElsewhere proves the
// strict check is not over-eager: a concurrent edit far from the anchor does
// not block a legitimate insert that did land contiguously after the anchor.
func TestDocsUpdateEmulatedMoveSucceedsDespiteConcurrentEditElsewhere(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-emulate-concurrent-ok"))

	// after: our rebuilt block lands right after the anchor AND an unrelated
	// new block appears at the top from a concurrent edit.
	afterXML := strings.Replace(emulateTestXML,
		`<p id="blkAnchor">anchor</p>`,
		`<p id="blkAnchor">anchor</p><p id="blkNew">First &amp; second</p>`, 1)
	afterXML = strings.Replace(afterXML,
		`<h1 id="blkHead">Title</h1>`,
		`<p id="blkConcurrent">unrelated</p><h1 id="blkHead">Title</h1>`, 1)

	registerEmulateFetchStub(reg, "doxcnEmu", emulateTestXML)
	registerEmulateUpdateStub(reg, "doxcnEmu", "block_insert_after")
	registerEmulateFetchStub(reg, "doxcnEmu", afterXML)
	registerEmulateUpdateStub(reg, "doxcnEmu", "block_delete")

	err := mountAndRunDocs(t, DocsUpdate, []string{
		"+update",
		"--doc", "doxcnEmu",
		"--command", "block_move_after",
		"--block-id", "blkAnchor",
		"--src-block-ids", "blkA",
		"--emulate",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	out := decodeDocEmulateOutput(t, stdout)
	if !out.Data.SrcBlocksDeleted {
		t.Fatal("move should delete the source blocks once anchor-scoped verification passes")
	}
	// Only the anchor-adjacent rebuilt block counts as ours; the concurrent
	// block must not be reported as a new block id.
	if len(out.Data.NewBlockIDs) != 1 || out.Data.NewBlockIDs[0] != "blkNew" {
		t.Fatalf("new_block_ids = %v, want [blkNew] (concurrent edit excluded)", out.Data.NewBlockIDs)
	}
}
