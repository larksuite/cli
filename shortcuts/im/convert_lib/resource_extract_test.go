// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"reflect"
	"testing"
)

func TestExtractResourceRefs(t *testing.T) {
	tests := []struct {
		name      string
		msgType   string
		raw       string
		messageID string
		want      []ResourceRef
	}{
		{name: "image", msgType: "image", raw: `{"image_key":"img_1"}`, messageID: "om_1", want: []ResourceRef{{MessageID: "om_1", Key: "img_1", Type: "image"}}},
		{name: "file", msgType: "file", raw: `{"file_key":"f_1"}`, messageID: "om_2", want: []ResourceRef{{MessageID: "om_2", Key: "f_1", Type: "file"}}},
		{name: "audio", msgType: "audio", raw: `{"file_key":"a_1","duration":1000}`, messageID: "om_3", want: []ResourceRef{{MessageID: "om_3", Key: "a_1", Type: "file"}}},
		{name: "video", msgType: "video", raw: `{"file_key":"v_1"}`, messageID: "om_4", want: []ResourceRef{{MessageID: "om_4", Key: "v_1", Type: "file"}}},
		{name: "media", msgType: "media", raw: `{"file_key":"m_1"}`, messageID: "om_5", want: []ResourceRef{{MessageID: "om_5", Key: "m_1", Type: "file"}}},
		{name: "sticker not extracted", msgType: "sticker", raw: `{"file_key":"s_1"}`, messageID: "om_6", want: nil},
		{name: "image without key skipped", msgType: "image", raw: `{}`, messageID: "om_7", want: nil},
		{name: "file without key skipped", msgType: "file", raw: `{}`, messageID: "om_8", want: nil},
		{name: "invalid json skipped", msgType: "image", raw: `{invalid`, messageID: "om_9", want: nil},
		{name: "text has no resource", msgType: "text", raw: `{"text":"hi"}`, messageID: "om_10", want: nil},
		{
			name:      "post img and media",
			msgType:   "post",
			raw:       `{"zh_cn":{"content":[[{"tag":"img","image_key":"post_img"},{"tag":"text","text":"x"}],[{"tag":"media","file_key":"post_media"}]]}}`,
			messageID: "om_11",
			want:      []ResourceRef{{MessageID: "om_11", Key: "post_img", Type: "image"}, {MessageID: "om_11", Key: "post_media", Type: "file"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractResourceRefs(tt.msgType, tt.raw, tt.messageID, nil)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ExtractResourceRefs(%s) = %#v, want %#v", tt.name, got, tt.want)
			}
		})
	}
}

// TestExtractMergeForwardSubItemRefs verifies merge_forward sub-item resources
// fold into the parent's ref list, each carrying the TOP-LEVEL container's
// message_id (NOT the sub-item's own id — the download API rejects sub-item ids
// with 234003 File not in msg), and that sticker sub-items are skipped.
func TestExtractMergeForwardSubItemRefs(t *testing.T) {
	mergeSub := map[string][]map[string]interface{}{
		"mf_1": {
			{"message_id": "sub_img", "msg_type": "image", "body": map[string]interface{}{"content": `{"image_key":"img_s"}`}},
			{"message_id": "sub_sticker", "msg_type": "sticker", "body": map[string]interface{}{"content": `{"file_key":"k"}`}},
			{"message_id": "sub_file", "msg_type": "file", "body": map[string]interface{}{"content": `{"file_key":"f_s"}`}},
		},
	}

	got := ExtractResourceRefs("merge_forward", "", "mf_1", mergeSub)
	want := []ResourceRef{
		{MessageID: "mf_1", Key: "img_s", Type: "image"},
		{MessageID: "mf_1", Key: "f_s", Type: "file"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractResourceRefs(merge_forward) = %#v, want %#v", got, want)
	}

	// No prefetch entry → no refs (sub-items unavailable, fail-silent).
	if got := ExtractResourceRefs("merge_forward", "", "mf_absent", mergeSub); got != nil {
		t.Fatalf("ExtractResourceRefs(merge_forward absent) = %#v, want nil", got)
	}
}

// TestExtractMergeForwardSubItemRefs_Cyclic is a regression test for a real
// stack overflow: a merge_forward's prefetched flat sub-item list can include
// the container's own id and/or a back-pointing merge_forward, which previously
// recursed until the stack blew up. The visited guard must expand each id once
// and still collect the leaf resources.
func TestExtractMergeForwardSubItemRefs_Cyclic(t *testing.T) {
	mergeSub := map[string][]map[string]interface{}{
		// mf_root lists ITSELF (container appears in its own sub-list) plus a leaf
		// and a nested merge_forward that points back to mf_root.
		"mf_root": {
			{"message_id": "mf_root", "msg_type": "merge_forward", "body": map[string]interface{}{"content": "[Merged forward]"}},
			{"message_id": "sub_img", "msg_type": "image", "body": map[string]interface{}{"content": `{"image_key":"img_c"}`}},
			{"message_id": "mf_child", "msg_type": "merge_forward", "body": map[string]interface{}{"content": "[Merged forward]"}},
		},
		"mf_child": {
			{"message_id": "mf_root", "msg_type": "merge_forward", "body": map[string]interface{}{"content": "[Merged forward]"}},
			{"message_id": "sub_file", "msg_type": "file", "body": map[string]interface{}{"content": `{"file_key":"file_c"}`}},
		},
	}

	// Must terminate (no stack overflow) and collect both leaf resources once,
	// each addressed by the top-level container id (mf_root) for download — even
	// the resource nested inside mf_child, since nested merge_forward ids are
	// virtual sub-items that cannot own a download either.
	got := ExtractResourceRefs("merge_forward", "", "mf_root", mergeSub)
	want := []ResourceRef{
		{MessageID: "mf_root", Key: "img_c", Type: "image"},
		{MessageID: "mf_root", Key: "file_c", Type: "file"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractResourceRefs(cyclic merge_forward) = %#v, want %#v", got, want)
	}
}
