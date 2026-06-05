// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs/localfileio"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestValidateMediaFlagPath(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)
	os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte("img"), 0644)

	fio := &localfileio.LocalFileIO{}

	tests := []struct {
		name    string
		flag    string
		value   string
		wantErr bool
	}{
		{"empty value skipped", "--image", "", false},
		{"http URL skipped", "--image", "http://example.com/a.jpg", false},
		{"https URL skipped", "--file", "https://example.com/b.mp4", false},
		{"media key skipped", "--image", "img_abc123", false},
		{"file key skipped", "--file", "file_abc123", false},
		{"valid local file", "--image", "photo.jpg", false},
		{"nonexistent file allowed", "--file", "missing.txt", false},
		{"path traversal rejected", "--image", "../../etc/passwd", true},
		{"absolute path rejected", "--file", "/etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMediaFlagPath(fio, tt.flag, tt.value)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %s=%q, got nil", tt.flag, tt.value)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for %s=%q: %v", tt.flag, tt.value, err)
			}
		})
	}
}

func TestIMMediaFlagDescriptionsDocumentPathRestrictions(t *testing.T) {
	shortcuts := []struct {
		name  string
		flags []common.Flag
	}{
		{name: "messages-send", flags: ImMessagesSend.Flags},
		{name: "messages-reply", flags: ImMessagesReply.Flags},
	}
	mediaFlags := []string{"image", "file", "video", "video-cover", "audio"}
	for _, sc := range shortcuts {
		for _, flagName := range mediaFlags {
			t.Run(sc.name+"/"+flagName, func(t *testing.T) {
				desc := findFlagDesc(t, sc.flags, flagName)
				for _, want := range []string{"URL", "cwd-relative local path", "absolute paths", ".. are rejected"} {
					if !strings.Contains(desc, want) {
						t.Fatalf("%s --%s description = %q, want it to mention %q", sc.name, flagName, desc, want)
					}
				}
			})
		}
	}
}

func findFlagDesc(t *testing.T, flags []common.Flag, name string) string {
	t.Helper()
	for _, flag := range flags {
		if flag.Name == name {
			return flag.Desc
		}
	}
	t.Fatalf("flag %q not found", name)
	return ""
}
