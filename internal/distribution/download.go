// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/larksuite/cli/internal/vfs"
)

// These ceilings bound temporary disk use while leaving ample room for the
// CLI and Skills bundles. Raise them only when a supported bundle outgrows
// the current distribution contract.
const artifactDownloadMaxBytes int64 = 4 << 30

func downloadArtifact(ctx context.Context, artifact Artifact, directory, pattern string) (string, error) {
	return downloadArtifactWithLimit(ctx, artifact, directory, pattern, artifactDownloadMaxBytes)
}

func downloadArtifactWithLimit(ctx context.Context, artifact Artifact, directory, pattern string, maxBytes int64) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return "", err
	}
	response, err := httpClient().Do(request)
	if err != nil {
		return "", redactRequestError(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", newHTTPStatusError("artifact download", response.StatusCode)
	}
	if response.ContentLength > maxBytes {
		return "", fmt.Errorf("artifact download exceeds %d bytes", maxBytes)
	}
	temporary, err := vfs.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	path := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = vfs.Remove(path)
		}
	}()

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return "", err
	}
	if written > maxBytes {
		return "", fmt.Errorf("artifact download exceeds %d bytes", maxBytes)
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	want := strings.TrimPrefix(artifact.Checksum, "sha256:")
	got := hex.EncodeToString(hash.Sum(nil))
	if got != want {
		return "", fmt.Errorf("artifact checksum mismatch")
	}
	keep = true
	return path, nil
}
