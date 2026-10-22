// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package localfileio

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/larksuite/cli/extension/fileio"
)

func TestAppendToRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := (&LocalFileIO{}).AppendTo(path, fileio.SaveOptions{}, neverReader{})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("AppendTo() error = %v, want a prompt FIFO rejection", err)
		}
	case <-time.After(time.Second):
		t.Fatal("AppendTo() blocked while opening a FIFO")
	}
}

type neverReader struct{}

func (neverReader) Read([]byte) (int, error) { panic("FIFO test reader must not be called") }
