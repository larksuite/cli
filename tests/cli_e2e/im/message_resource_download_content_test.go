// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import "testing"

// TestFileKeyFromMessageContent pins the shape `im +messages-mget` actually
// returns. The live workflow first read the key with gjson, assuming raw
// platform JSON; the shortcut converts content to its display form instead, so
// that lookup silently returned an empty key and the download never ran.
func TestFileKeyFromMessageContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantOK  bool
	}{
		{
			name:    "converted file message",
			content: `<file key="file_v3_sample_resource_bin" name="resource.bin"/>`,
			want:    "file_v3_sample_resource_bin",
			wantOK:  true,
		},
		{
			name:    "converted image message",
			content: `<img key="img_v3_sample_image_png" />`,
			want:    "img_v3_sample_image_png",
			wantOK:  true,
		},
		{
			name:    "raw platform json is not what mget returns",
			content: `{"file_key":"file_v3_x","file_name":"resource.bin"}`,
			wantOK:  false,
		},
		{name: "text message has no key", content: "hello", wantOK: false},
		{name: "empty content", content: "", wantOK: false},
		{name: "empty key attribute", content: `<file key="" name="resource.bin"/>`, wantOK: false},
		{name: "unterminated key attribute", content: `<file key="file_v3_x`, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := fileKeyFromMessageContent(tt.content)
			if ok != tt.wantOK {
				t.Fatalf("fileKeyFromMessageContent() ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("fileKeyFromMessageContent() = %q, want %q", got, tt.want)
			}
		})
	}
}
