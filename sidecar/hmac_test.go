// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sidecar

import (
	"strconv"
	"testing"
	"time"
)

func TestBodySHA256_Empty(t *testing.T) {
	// SHA-256 of empty string is a well-known constant.
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := BodySHA256(nil); got != want {
		t.Errorf("BodySHA256(nil) = %q, want %q", got, want)
	}
	if got := BodySHA256([]byte{}); got != want {
		t.Errorf("BodySHA256([]byte{}) = %q, want %q", got, want)
	}
}

func TestBodySHA256_NonEmpty(t *testing.T) {
	got := BodySHA256([]byte(`{"key":"value"}`))
	if len(got) != 64 {
		t.Errorf("expected 64-char hex string, got %d chars", len(got))
	}
}

func TestSignAndVerify(t *testing.T) {
	key := []byte("test-secret-key-32bytes-long!!!!!")
	method := "POST"
	host := "open.feishu.cn"
	pathAndQuery := "/open-apis/im/v1/messages?receive_id_type=chat_id"
	body := []byte(`{"content":"hello"}`)
	bodySHA := BodySHA256(body)
	ts := Timestamp()

	sig := Sign(key, method, host, pathAndQuery, bodySHA, ts)
	if len(sig) != 64 {
		t.Fatalf("signature should be 64-char hex, got %d chars", len(sig))
	}

	// Valid verification
	if err := Verify(key, method, host, pathAndQuery, bodySHA, ts, sig); err != nil {
		t.Fatalf("Verify failed for valid signature: %v", err)
	}

	// Wrong key
	if err := Verify([]byte("wrong-key"), method, host, pathAndQuery, bodySHA, ts, sig); err == nil {
		t.Error("Verify should fail with wrong key")
	}

	// Tampered method
	if err := Verify(key, "GET", host, pathAndQuery, bodySHA, ts, sig); err == nil {
		t.Error("Verify should fail with tampered method")
	}

	// Tampered host
	if err := Verify(key, method, "evil.com", pathAndQuery, bodySHA, ts, sig); err == nil {
		t.Error("Verify should fail with tampered host")
	}

	// Tampered body hash
	if err := Verify(key, method, host, pathAndQuery, BodySHA256([]byte("tampered")), ts, sig); err == nil {
		t.Error("Verify should fail with tampered body hash")
	}
}

func TestVerify_TimestampDrift(t *testing.T) {
	key := []byte("test-key")
	bodySHA := BodySHA256(nil)

	// Timestamp too old
	oldTs := strconv.FormatInt(time.Now().Unix()-MaxTimestampDrift-10, 10)
	sig := Sign(key, "GET", "host", "/path", bodySHA, oldTs)
	if err := Verify(key, "GET", "host", "/path", bodySHA, oldTs, sig); err == nil {
		t.Error("Verify should reject expired timestamp")
	}

	// Timestamp too far in future
	futureTs := strconv.FormatInt(time.Now().Unix()+MaxTimestampDrift+10, 10)
	sig = Sign(key, "GET", "host", "/path", bodySHA, futureTs)
	if err := Verify(key, "GET", "host", "/path", bodySHA, futureTs, sig); err == nil {
		t.Error("Verify should reject future timestamp")
	}

	// Invalid timestamp
	if err := Verify(key, "GET", "host", "/path", bodySHA, "not-a-number", "sig"); err == nil {
		t.Error("Verify should reject invalid timestamp")
	}
}

func TestSignDeterministic(t *testing.T) {
	key := []byte("key")
	a := Sign(key, "GET", "host", "/path", "abc", "12345")
	b := Sign(key, "GET", "host", "/path", "abc", "12345")
	if a != b {
		t.Error("Sign should be deterministic")
	}
}

func TestValidateProxyAddr(t *testing.T) {
	valid := []string{
		"http://127.0.0.1:16384",
		"https://127.0.0.1:16384",
		"http://localhost:8080",
		"http://host.docker.internal:16384",
		"http://0.0.0.0:8080/",
		"127.0.0.1:16384",
		"localhost:8080",
		"[::1]:16384",
	}
	for _, addr := range valid {
		if err := ValidateProxyAddr(addr); err != nil {
			t.Errorf("ValidateProxyAddr(%q) unexpected error: %v", addr, err)
		}
	}

	invalid := []string{
		"",
		"foobar",
		"ftp://127.0.0.1:16384",
		"http://",
		"http://127.0.0.1:16384/some/path",
		":16384",
	}
	for _, addr := range invalid {
		if err := ValidateProxyAddr(addr); err == nil {
			t.Errorf("ValidateProxyAddr(%q) expected error, got nil", addr)
		}
	}
}

func TestProxyHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"http://127.0.0.1:16384", "127.0.0.1:16384"},
		{"http://0.0.0.0:8080", "0.0.0.0:8080"},
		{"http://host.docker.internal:16384/", "host.docker.internal:16384"},
		{"127.0.0.1:16384", "127.0.0.1:16384"}, // no scheme
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ProxyHost(tt.input); got != tt.want {
				t.Errorf("ProxyHost(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
