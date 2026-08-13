// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

// Package extendedupdate updates an Extended binary from the matching
// lark-cli-extended asset in GitHub Releases.
package extendedupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/transport"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	latestReleaseURL = "https://api.github.com/repos/larksuite/cli/releases/latest"
	releaseBaseURL   = "https://github.com/larksuite/cli/releases/download"
	maxChecksumBytes = 1 << 20
	maxArchiveBytes  = 256 << 20
	requestTimeout   = 2 * time.Minute
	verifyTimeout    = 10 * time.Second
)

var httpClient = newHTTPClient()

type release struct {
	TagName string `json:"tag_name"`
}

type versionInfo struct {
	Version string `json:"version"`
	Edition string `json:"edition"`
}

func newHTTPClient() *http.Client {
	client := transport.NewHTTPClient(requestTimeout)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || !trustedDownloadHost(req.URL.Hostname()) {
			return fmt.Errorf("release download redirected to an untrusted URL: %s", req.URL.Redacted())
		}
		if len(via) >= 5 {
			return fmt.Errorf("release download exceeded redirect limit")
		}
		return nil
	}
	return client
}

func trustedDownloadHost(host string) bool {
	host = strings.ToLower(host)
	return host == "github.com" ||
		host == "api.github.com" ||
		strings.HasSuffix(host, ".githubusercontent.com")
}

// FetchLatest returns the latest release version. Install verifies that the
// matching Extended asset and checksum entry exist before replacing anything.
func FetchLatest() (string, error) {
	body, err := download(latestReleaseURL, maxChecksumBytes)
	if err != nil {
		return "", errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"failed to query the latest Extended release: %v", err).WithCause(err)
	}
	var latest release
	if err := json.Unmarshal(body, &latest); err != nil {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse,
			"GitHub returned an invalid latest release response").WithCause(err)
	}
	version := strings.TrimPrefix(strings.TrimSpace(latest.TagName), "v")
	if version == "" || strings.ContainsAny(version, `/\`) {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse,
			"GitHub returned an invalid latest release tag")
	}
	return version, nil
}

// Install downloads, verifies, and atomically replaces the running Extended
// binary with the requested Extended release asset.
func Install(version string) error {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	archiveName, err := assetName(version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "%v", err).WithCause(err)
	}
	base := releaseBaseURL + "/v" + version
	checksums, err := download(base+"/checksums.txt", maxChecksumBytes)
	if err != nil {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"failed to download Extended release checksums: %v", err).WithCause(err)
	}
	expected, err := checksumFor(checksums, archiveName)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"Extended release checksum is invalid: %v", err).WithCause(err)
	}
	archive, err := download(base+"/"+archiveName, maxArchiveBytes)
	if err != nil {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"failed to download Extended release asset: %v", err).WithCause(err)
	}
	actual := sha256.Sum256(archive)
	if !bytes.Equal(actual[:], expected) {
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"Extended release checksum verification failed")
	}
	binary, err := extractBinary(archive, runtime.GOOS)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"Extended release archive is invalid: %v", err).WithCause(err)
	}
	if err := replaceCurrent(binary, version); err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO,
			"failed to install lark-cli Extended: %v", err).
			WithCause(err).
			WithHint("ensure the current lark-cli installation directory is writable")
	}
	return nil
}

func download(rawURL string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "lark-cli-extended")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return body, nil
}

func assetName(version, goos, goarch string) (string, error) {
	switch goos {
	case "darwin", "linux", "windows":
	default:
		return "", fmt.Errorf("Extended update does not support %s", goos)
	}
	switch goarch {
	case "amd64", "arm64", "riscv64":
	default:
		return "", fmt.Errorf("Extended update does not support %s/%s", goos, goarch)
	}
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("lark-cli-extended-%s-%s-%s%s", version, goos, goarch, ext), nil
}

func checksumFor(data []byte, asset string) ([]byte, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != asset {
			continue
		}
		sum, err := hex.DecodeString(fields[0])
		if err != nil || len(sum) != sha256.Size {
			return nil, fmt.Errorf("invalid SHA-256 for %s", asset)
		}
		return sum, nil
	}
	return nil, fmt.Errorf("checksums.txt does not contain %s", asset)
}

func extractBinary(archive []byte, goos string) ([]byte, error) {
	name := "lark-cli"
	if goos == "windows" {
		name += ".exe"
		reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, file := range reader.File {
			if path.Clean(file.Name) != name || file.FileInfo().IsDir() {
				continue
			}
			rc, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return readBinary(rc)
		}
		return nil, fmt.Errorf("%s is missing", name)
	}

	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if path.Clean(header.Name) == name && header.Typeflag == tar.TypeReg {
			return readBinary(tr)
		}
	}
	return nil, fmt.Errorf("%s is missing", name)
}

func readBinary(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxArchiveBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || int64(len(data)) > maxArchiveBytes {
		return nil, fmt.Errorf("binary has invalid size")
	}
	return data, nil
}

func replaceCurrent(binary []byte, version string) error {
	exe, err := vfs.Executable()
	if err != nil {
		return err
	}
	exe, err = vfs.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	tmp, err := vfs.CreateTemp(dir, ".lark-cli-extended-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer vfs.Remove(tmpName)
	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := verifyBinary(tmpName, version); err != nil {
		return err
	}

	backup := exe + ".old"
	_ = vfs.Remove(backup)
	if err := vfs.Rename(exe, backup); err != nil {
		return err
	}
	restore := func() {
		_ = vfs.Remove(exe)
		_ = vfs.Rename(backup, exe)
	}
	if err := vfs.Rename(tmpName, exe); err != nil {
		restore()
		return err
	}
	if err := verifyBinary(exe, version); err != nil {
		restore()
		return err
	}
	_ = vfs.Remove(backup)
	return nil
}

func verifyBinary(exe, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, "version", "--json").Output()
	if err != nil {
		return fmt.Errorf("candidate binary is not executable: %w", err)
	}
	var info versionInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return fmt.Errorf("candidate returned invalid version metadata: %w", err)
	}
	if strings.TrimPrefix(info.Version, "v") != strings.TrimPrefix(version, "v") {
		return fmt.Errorf("candidate version is %q, want %q", info.Version, version)
	}
	if info.Edition != "extended" {
		return fmt.Errorf("candidate edition is %q, want extended", info.Edition)
	}
	return nil
}
