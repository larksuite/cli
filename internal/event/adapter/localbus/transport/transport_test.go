// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package transport

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUnixTransport_Address(t *testing.T) {
	tr := New()
	addr := tr.Address("cli_test123")
	if addr == "" {
		t.Fatal("address should not be empty")
	}
	if !contains(addr, "cli_test123") {
		t.Errorf("address %q should contain appID", addr)
	}
}

func TestUnixTransport_ListenAndDial(t *testing.T) {
	tr := New()
	dir := t.TempDir()
	addr := filepath.Join(dir, "t.sock") // macOS unix socket path limit is 103 bytes

	ln, err := tr.Listen(addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	conn, err := tr.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	serverConn := <-accepted
	defer serverConn.Close()

	_, err = conn.Write([]byte("hello\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 64)
	n, err := serverConn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != "hello\n" {
		t.Errorf("got %q, want %q", string(buf[:n]), "hello\n")
	}
}

func TestUnixTransport_ListenTwiceFails(t *testing.T) {
	tr := New()
	dir := t.TempDir()
	addr := filepath.Join(dir, "s")

	ln1, err := tr.Listen(addr)
	if err != nil {
		t.Fatalf("first listen: %v", err)
	}
	defer ln1.Close()

	_, err = tr.Listen(addr)
	if err == nil {
		t.Error("second listen on same addr should fail")
	}
}

func TestUnixTransport_Cleanup(t *testing.T) {
	tr := New()
	dir := t.TempDir()
	addr := filepath.Join(dir, "t.sock") // macOS unix socket path limit is 103 bytes

	ln, _ := tr.Listen(addr)
	ln.Close()
	tr.Cleanup(addr)

	if _, err := os.Stat(addr); !os.IsNotExist(err) {
		t.Error("sock file should be removed after Cleanup")
	}
}

// Dial must fail-fast or honor 5s timeout when nothing is listening — never block forever.
func TestUnixDialTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "n.sock")

	os.Remove(sockPath)

	tr := &unixTransport{}
	start := time.Now()
	conn, err := tr.Dial(sockPath)
	elapsed := time.Since(start)

	if err == nil {
		conn.Close()
		t.Fatal("expected error dialing non-listening socket")
	}
	if elapsed > 6*time.Second {
		t.Errorf("Dial took %v; should fail-fast or honor 5s timeout", elapsed)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
