// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadArtifactRejectsExcessiveBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("123456789"))
	}))
	defer server.Close()

	_, err := downloadArtifactWithLimit(context.Background(), Artifact{
		URL: server.URL, Checksum: testChecksum,
	}, t.TempDir(), "artifact-*", 8)
	if err == nil || !strings.Contains(err.Error(), "exceeds 8 bytes") {
		t.Fatalf("err = %v", err)
	}
}

func TestDownloadArtifactDoesNotApplyManifestDeadline(t *testing.T) {
	payload := []byte("artifact")
	previousClient := DefaultClient
	DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if _, ok := req.Context().Deadline(); ok {
			t.Fatal("artifact request inherited the manifest deadline")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(string(payload))),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { DefaultClient = previousClient })
	checksum := fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
	if _, err := downloadArtifact(context.Background(), Artifact{URL: "https://dist.example/artifact", Checksum: checksum}, t.TempDir(), "artifact-*"); err != nil {
		t.Fatal(err)
	}
}
