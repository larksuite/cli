// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
)

// downloadMaxBytes caps the artifact size we'll accept. Comfortably over the
// real lark-sec-cli zip (~tens of MB) and well under what a malicious mirror
// could use to exhaust local disk before we noticed.
const downloadMaxBytes = 512 * 1024 * 1024

// DownloadOptions controls Download.
type DownloadOptions struct {
	URL         string
	Destination string // full path to the .zip we'll create
	HTTPClient  *http.Client

	// ExpectedSHA256, if non-empty, is the hex SHA256 the artifact MUST
	// match — verified after the full body has been streamed. Use this when
	// the manifest publishes a hash for the artifact (e.g. bootstrap.json's
	// `extra.sha256`). Any mismatch fails the download with the .part file
	// removed.
	//
	// When empty (the manifest doesn't carry a hash), the only integrity
	// check left is the CDN's own `Content-MD5` response header, applied
	// opportunistically below.
	ExpectedSHA256 string
}

// Download streams URL to Destination. Writes to a sibling .part file and
// renames into place on success so a crashed or aborted run leaves no
// half-written zip the next run might mistake for valid.
//
// Two layers of integrity check, both opt-in:
//
//  1. ExpectedSHA256 (strong, manifest-provided): cryptographic, fails the
//     download on mismatch. Use whenever the release manifest carries a hash.
//  2. CDN `Content-MD5` header (opportunistic): non-cryptographic, catches
//     edge replacement or transit corruption when the upstream CDN populates
//     the header. Runs unconditionally — if the header is present we honour it.
//
// Neither check defends against a malicious upstream that controls both the
// artifact AND the manifest. That class of risk has to be handled by signing
// the release pipeline, which is out of scope for the client.
func Download(ctx context.Context, opts DownloadOptions) error {
	if opts.HTTPClient == nil {
		return fmt.Errorf("Download: HTTPClient is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return err
	}
	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", opts.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: status %d", opts.URL, resp.StatusCode)
	}

	tmpPath := opts.Destination + ".part"
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	cleanup := func() { out.Close(); os.Remove(tmpPath) }

	// Hash both ways during the single read pass. Both hashers are cheap and
	// we don't know yet which check (or both) we'll actually need.
	sha := sha256.New()
	md := md5.New()
	writer := io.MultiWriter(out, sha, md)

	n, err := io.Copy(writer, io.LimitReader(resp.Body, downloadMaxBytes+1))
	if err != nil {
		cleanup()
		return fmt.Errorf("download %s: %w", opts.URL, err)
	}
	if n > downloadMaxBytes {
		cleanup()
		return fmt.Errorf("download %s: exceeds %d bytes", opts.URL, downloadMaxBytes)
	}
	if err := out.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := verifyChecksums(resp, opts.ExpectedSHA256, sha, md); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("download %s: %w", opts.URL, err)
	}

	if err := os.Rename(tmpPath, opts.Destination); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// verifyChecksums applies the two-layer integrity check after the body has
// been fully streamed. Returns nil when both layers (whichever apply) agree.
func verifyChecksums(resp *http.Response, expectedSHA256 string, sha, md hash.Hash) error {
	if expectedSHA256 != "" {
		got := hex.EncodeToString(sha.Sum(nil))
		if !equalFoldHex(got, expectedSHA256) {
			return fmt.Errorf("sha256 mismatch: expected %s, got %s", expectedSHA256, got)
		}
	}

	if cdnMD5 := resp.Header.Get("Content-MD5"); cdnMD5 != "" {
		got := base64.StdEncoding.EncodeToString(md.Sum(nil))
		if got != cdnMD5 {
			return fmt.Errorf("content-md5 mismatch: cdn=%s, computed=%s", cdnMD5, got)
		}
	}
	return nil
}

// equalFoldHex is a non-allocating ASCII case-insensitive compare for hex
// strings. SHA256 manifests sometimes ship uppercase, sometimes lowercase.
func equalFoldHex(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
