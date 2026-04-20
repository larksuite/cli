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

func TestDetectAttachmentMIMEFromContentBinaryFallback(t *testing.T) {
	got := detectAttachmentMIMEFromContent([]byte{0x00, 0x01, 0x02, 0x03})
	if got != "application/octet-stream" {
		t.Fatalf("detectAttachmentMIMEFromContent() = %q, want %q", got, "application/octet-stream")
	}
}
