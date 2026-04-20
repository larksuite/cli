// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSerializeGoldenFixtures(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		golden  string
		patch   Patch
		patchFn func(*testing.T) Patch
	}{
		{
			name:   "reply-subject",
			input:  "testdata/reply_draft.eml",
			golden: "testdata/reply_draft_subject.golden.eml",
			patch:  Patch{Ops: []PatchOp{{Op: "set_subject", Value: "Updated reply"}}},
		},
		{
			name:   "alternative-set-body",
			input:  "testdata/alternative_draft.eml",
			golden: "testdata/alternative_set_body.golden.eml",
			patch:  Patch{Ops: []PatchOp{{Op: "set_body", Value: "<div>updated <strong>body</strong></div>"}}},
		},
		{
			name:   "html-inline-replace",
			input:  "testdata/html_inline_draft.eml",
			golden: "testdata/html_inline_replace.golden.eml",
			patch:  Patch{Ops: []PatchOp{{Op: "replace_body", BodyKind: "text/html", Selector: "primary", Value: `<div>updated<img src="cid:logo"></div>`}}},
		},
		{
			name:   "forward-remove-attachment",
			input:  "testdata/forward_draft.eml",
			golden: "testdata/forward_remove_attachment.golden.eml",
			patch:  Patch{Ops: []PatchOp{{Op: "remove_attachment", Target: AttachmentTarget{PartID: "1.3"}}}},
		},
		{
			name:   "custom-header-preserved",
			input:  "testdata/custom_header_draft.eml",
			golden: "testdata/custom_header_set_subject.golden.eml",
			patch:  Patch{Ops: []PatchOp{{Op: "set_subject", Value: "Updated custom"}}},
		},
		{
			name:   "inline-replace-binary",
			input:  "testdata/html_inline_draft.eml",
			golden: "testdata/html_inline_replace_binary.golden.eml",
			patchFn: func(t *testing.T) Patch {
				t.Helper()
				chdirTemp(t)
				if err := os.WriteFile("updated-inline.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
					t.Fatalf("WriteFile error = %v", err)
				}
				return Patch{Ops: []PatchOp{{Op: "replace_inline", Target: AttachmentTarget{PartID: "1.2"}, Path: "updated-inline.png"}}}
			},
		},
		{
			name:   "inline-remove-with-html-update",
			input:  "testdata/html_inline_draft.eml",
			golden: "testdata/html_inline_remove.golden.eml",
			patch: Patch{Ops: []PatchOp{
				{Op: "replace_body", BodyKind: "text/html", Selector: "primary", Value: `<div>updated without image</div>`},
				{Op: "remove_inline", Target: AttachmentTarget{PartID: "1.2"}},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Read fixture files before patchFn which may chdirTemp.
			input := mustReadFixture(t, tc.input)
			want := mustReadFixture(t, tc.golden)
			snapshot := mustParseFixtureDraft(t, input)
			patch := tc.patch
			if tc.patchFn != nil {
				patch = tc.patchFn(t)
			}
			if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, patch); err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			raw, err := Serialize(snapshot)
			if err != nil {
				t.Fatalf("Serialize() error = %v", err)
			}
			decoded, err := decodeRawEML(raw)
			if err != nil {
				t.Fatalf("decodeRawEML() error = %v", err)
			}
			got := string(decoded)
			if got != want {
				t.Fatalf("golden mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
			}
		})
	}
}

func mustReadFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}
