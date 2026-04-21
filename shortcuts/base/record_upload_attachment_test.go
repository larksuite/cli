// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/fileio"
)

type attachmentTestFileIO struct {
	openFile fileio.File
	openErr  error
}

func (f attachmentTestFileIO) Open(string) (fileio.File, error) { return f.openFile, f.openErr }
func (attachmentTestFileIO) Stat(string) (fileio.FileInfo, error) {
	return attachmentTestFileInfo{}, nil
}
func (attachmentTestFileIO) ResolvePath(path string) (string, error) { return path, nil }
func (attachmentTestFileIO) Save(string, fileio.SaveOptions, io.Reader) (fileio.SaveResult, error) {
	return nil, nil
}

type attachmentTestFileInfo struct{}

func (attachmentTestFileInfo) Size() int64       { return 0 }
func (attachmentTestFileInfo) IsDir() bool       { return false }
func (attachmentTestFileInfo) Mode() fs.FileMode { return 0 }

type attachmentTestFile struct {
	*bytes.Reader
}

func newAttachmentTestFile(content []byte) attachmentTestFile {
	return attachmentTestFile{Reader: bytes.NewReader(content)}
}

func (attachmentTestFile) Close() error { return nil }

type attachmentReadErrorFile struct{}

func (attachmentReadErrorFile) Read([]byte) (int, error)          { return 0, os.ErrPermission }
func (attachmentReadErrorFile) ReadAt([]byte, int64) (int, error) { return 0, io.EOF }
func (attachmentReadErrorFile) Close() error                      { return nil }

func TestDetectAttachmentMIMETypeUsesExtension(t *testing.T) {
	got, err := detectAttachmentMIMEType(nil, "ignored", "note.TXT")
	if err != nil {
		t.Fatalf("detectAttachmentMIMEType() error = %v", err)
	}
	if got != "text/plain" {
		t.Fatalf("detectAttachmentMIMEType() = %q, want %q", got, "text/plain")
	}
}

func TestDetectAttachmentMIMETypeFallsBackToSourcePathExtension(t *testing.T) {
	got, err := detectAttachmentMIMEType(nil, "report.docx", "report")
	if err != nil {
		t.Fatalf("detectAttachmentMIMEType() error = %v", err)
	}
	if got != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("detectAttachmentMIMEType() = %q, want docx MIME type", got)
	}
}

func TestDetectAttachmentMIMETypeFallsBackToContent(t *testing.T) {
	fio := attachmentTestFileIO{openFile: newAttachmentTestFile([]byte("hello from base attachment"))}

	got, err := detectAttachmentMIMEType(fio, "note", "note")
	if err != nil {
		t.Fatalf("detectAttachmentMIMEType() error = %v", err)
	}
	if got != "text/plain" {
		t.Fatalf("detectAttachmentMIMEType() = %q, want %q", got, "text/plain")
	}
}

func TestDetectAttachmentMIMETypeWrapsOpenError(t *testing.T) {
	fio := attachmentTestFileIO{openErr: os.ErrNotExist}

	_, err := detectAttachmentMIMEType(fio, "missing", "missing")
	if err == nil {
		t.Fatal("expected error for open failure")
	}
	if !strings.Contains(err.Error(), "cannot read file") {
		t.Fatalf("error = %v, want wrapped read failure", err)
	}
}

func TestDetectAttachmentMIMETypeReturnsReadError(t *testing.T) {
	fio := attachmentTestFileIO{openFile: attachmentReadErrorFile{}}

	_, err := detectAttachmentMIMEType(fio, "broken", "broken")
	if err == nil {
		t.Fatal("expected error for read failure")
	}
	if !strings.Contains(err.Error(), "cannot read file") {
		t.Fatalf("error = %v, want read failure", err)
	}
}

func TestDetectAttachmentMIMEFromContent(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    string
	}{
		{name: "empty", content: nil, want: "application/octet-stream"},
		{name: "png", content: []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, want: "image/png"},
		{name: "jpeg", content: []byte{0xff, 0xd8, 0xff, 0xe0}, want: "image/jpeg"},
		{name: "gif87a", content: []byte("GIF87a"), want: "image/gif"},
		{name: "gif89a", content: []byte("GIF89a"), want: "image/gif"},
		{name: "webp", content: []byte("RIFF1234WEBP"), want: "image/webp"},
		{name: "pdf", content: []byte("%PDF-1.7"), want: "application/pdf"},
		{name: "text", content: []byte("hello from base attachment"), want: "text/plain"},
		{name: "text with newline", content: []byte("hello\nworld\tok"), want: "text/plain"},
		{name: "control bytes", content: []byte{'h', 'i', 0x00}, want: "application/octet-stream"},
		{name: "binary fallback", content: []byte{0x00, 0x01, 0x02, 0x03}, want: "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectAttachmentMIMEFromContent(tt.content)
			if got != tt.want {
				t.Fatalf("detectAttachmentMIMEFromContent() = %q, want %q", got, tt.want)
			}
		})
	}
}
