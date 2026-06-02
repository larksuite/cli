// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const bodyContent = "lark-sec-cli pretend zip bytes"

// fixtureSHA256 / fixtureMD5 are the hashes of bodyContent.
var fixtureSHA256 string
var fixtureMD5b64 string

func init() {
	sum := sha256.Sum256([]byte(bodyContent))
	fixtureSHA256 = hex.EncodeToString(sum[:])
	m := md5.Sum([]byte(bodyContent))
	fixtureMD5b64 = base64.StdEncoding.EncodeToString(m[:])
}

func newFixtureServer(t *testing.T, setContentMD5 bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if setContentMD5 {
			w.Header().Set("Content-MD5", fixtureMD5b64)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(bodyContent))
	}))
}

// TestDownload_HappyPath_NoChecksum confirms that a download with no manifest
// SHA and no CDN MD5 succeeds — the integrity hooks are opt-in, not required.
func TestDownload_HappyPath_NoChecksum(t *testing.T) {
	srv := newFixtureServer(t, false)
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.zip")
	err := Download(context.Background(), DownloadOptions{
		URL:         srv.URL,
		Destination: dst,
		HTTPClient:  srv.Client(),
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != bodyContent {
		t.Errorf("body roundtrip mismatch")
	}
}

// TestDownload_SHA256_Match confirms the manifest-provided SHA256 path
// passes for a correct hash. Tests both cases (with and without CDN MD5)
// so the second layer doesn't interfere.
func TestDownload_SHA256_Match(t *testing.T) {
	for _, withMD5 := range []bool{false, true} {
		name := "noMD5"
		if withMD5 {
			name = "withCDNMd5"
		}
		t.Run(name, func(t *testing.T) {
			srv := newFixtureServer(t, withMD5)
			defer srv.Close()
			dst := filepath.Join(t.TempDir(), "out.zip")
			err := Download(context.Background(), DownloadOptions{
				URL:            srv.URL,
				Destination:    dst,
				HTTPClient:     srv.Client(),
				ExpectedSHA256: fixtureSHA256,
			})
			if err != nil {
				t.Fatalf("Download: %v", err)
			}
		})
	}
}

// TestDownload_SHA256_Mismatch is the safety property: a wrong manifest hash
// rejects the download AND removes the .part file so the next run doesn't
// pick up a poisoned zip.
func TestDownload_SHA256_Mismatch(t *testing.T) {
	srv := newFixtureServer(t, false)
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.zip")
	err := Download(context.Background(), DownloadOptions{
		URL:            srv.URL,
		Destination:    dst,
		HTTPClient:     srv.Client(),
		ExpectedSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err == nil {
		t.Fatal("expected sha256 mismatch error")
	}
	if !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Errorf("error should mention sha256 mismatch: %v", err)
	}
	if _, statErr := os.Stat(dst); statErr == nil {
		t.Errorf("dst should not exist after mismatch")
	}
	if _, statErr := os.Stat(dst + ".part"); statErr == nil {
		t.Errorf(".part should not exist after mismatch")
	}
}

// TestDownload_ContentMD5_Mismatch confirms the opportunistic check fires
// even when no manifest SHA was provided. Catches a CDN edge that returned
// content but a stale/wrong Content-MD5 header (or a poisoned proxy).
func TestDownload_ContentMD5_Mismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-MD5", "Z3JhZmFuYTpyZWFsbHk/Pz8/Pz8/PzA9PT0=") // arbitrary
		_, _ = w.Write([]byte(bodyContent))
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.zip")
	err := Download(context.Background(), DownloadOptions{
		URL:         srv.URL,
		Destination: dst,
		HTTPClient:  srv.Client(),
	})
	if err == nil {
		t.Fatal("expected content-md5 mismatch error")
	}
	if !strings.Contains(err.Error(), "content-md5 mismatch") {
		t.Errorf("error should mention content-md5 mismatch: %v", err)
	}
}

// TestDownload_SHA256_CaseInsensitive guards the hex compare against case
// drift in the manifest (some publishers upper-case).
func TestDownload_SHA256_CaseInsensitive(t *testing.T) {
	srv := newFixtureServer(t, false)
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.zip")
	err := Download(context.Background(), DownloadOptions{
		URL:            srv.URL,
		Destination:    dst,
		HTTPClient:     srv.Client(),
		ExpectedSHA256: strings.ToUpper(fixtureSHA256),
	})
	if err != nil {
		t.Fatalf("Download (uppercase sha): %v", err)
	}
}

// TestDownload_404_NoPartFile confirms that a non-200 response leaves no
// .part file behind to confuse the next attempt.
func TestDownload_404_NoPartFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.zip")
	err := Download(context.Background(), DownloadOptions{
		URL:         srv.URL,
		Destination: dst,
		HTTPClient:  srv.Client(),
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if _, statErr := os.Stat(dst + ".part"); statErr == nil {
		t.Errorf(".part should not exist after 404")
	}
}
