// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/skillscheck"
	"github.com/larksuite/cli/internal/vfs"
)

func TestInstallDownloadsAndCommitsManifestArtifacts(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	executable := filepath.Join(root, "bin", "lark-cli")
	mustWrite(t, executable, "old binary")

	executableName := "lark-cli"
	if runtime.GOOS == "windows" {
		executableName += ".exe"
	}
	binaryArchive := buildTestZip(t, map[string]testZipFile{executableName: {content: "new binary", mode: 0o755}})
	skillsArchive := buildTestZip(t, map[string]testZipFile{
		"README.md":                        {content: "bundle metadata"},
		"lark-alpha/SKILL.md":              {content: "alpha"},
		"lark-alpha/references/guide.md":   {content: "guide"},
		"lark-alpha/scripts/check-install": {content: "#!/bin/sh\n", mode: 0o755},
		"lark-beta/SKILL.md":               {content: "beta"},
	})
	payloads := map[string][]byte{
		"/cli.zip":    binaryArchive,
		"/skills.zip": skillsArchive,
	}
	previousClient := DefaultClient
	DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		payload, ok := payloads[req.URL.Path]
		if !ok {
			return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: make(http.Header)}, nil
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          io.NopCloser(bytes.NewReader(payload)),
			ContentLength: int64(len(payload)),
			Header:        make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { DefaultClient = previousClient })

	manifest := &Manifest{
		Version:        "release-channel-7",
		sourceIdentity: "test-manifest",
		Artifacts: map[string]Artifact{
			CurrentPlatformKey(): {URL: "https://dist.example/cli.zip", Checksum: checksumFor(binaryArchive)},
			SkillsKey:            {URL: "https://dist.example/skills.zip", Checksum: checksumFor(skillsArchive)},
		},
	}
	skillsDir := filepath.Join(root, "skills")
	err := Install(context.Background(), manifest, InstallOptions{
		ExecutablePath: executable,
		SkillsDir:      skillsDir,
		VerifyBinary: func(path, version string) error {
			if version != manifest.Version {
				return fmt.Errorf("version = %q", version)
			}
			content, err := vfs.ReadFile(path)
			if err != nil {
				return err
			}
			if string(content) != "new binary" {
				return fmt.Errorf("binary = %q", content)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFile(t, executable, "new binary")
	assertFile(t, filepath.Join(skillsDir, "lark-alpha", "SKILL.md"), "alpha")
	assertFile(t, filepath.Join(skillsDir, "lark-alpha", "references", "guide.md"), "guide")
	assertFile(t, filepath.Join(skillsDir, "lark-beta", "SKILL.md"), "beta")
	script := filepath.Join(skillsDir, "lark-alpha", "scripts", "check-install")
	info, statErr := os.Stat(script)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("script mode = %v, want executable", info.Mode().Perm())
	}
	state, ok, readErr := skillscheck.ReadState()
	if readErr != nil || !ok || state.SourceIdentity != "test-manifest" {
		t.Fatalf("Skills state = %#v, %v, %v", state, ok, readErr)
	}
}

func TestInstallRejectsChecksumMismatchBeforeBinaryVerification(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	executableName := "lark-cli"
	if runtime.GOOS == "windows" {
		executableName += ".exe"
	}
	archive := buildTestZip(t, map[string]testZipFile{executableName: {content: "new binary"}})
	previousClient := DefaultClient
	DefaultClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(archive)),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { DefaultClient = previousClient })
	manifest := &Manifest{Version: "target", Artifacts: map[string]Artifact{
		CurrentPlatformKey(): {URL: "https://distribution.example/cli.zip", Checksum: "sha256:" + strings.Repeat("0", 64)},
		SkillsKey:            {URL: "https://distribution.example/skills.zip", Checksum: checksumFor(archive)},
	}}
	verified := false
	err := Install(context.Background(), manifest, InstallOptions{
		ExecutablePath: filepath.Join(root, executableName),
		VerifyBinary: func(string, string) error {
			verified = true
			return nil
		},
	})
	if err == nil || errors.Unwrap(err) == nil || !strings.Contains(errors.Unwrap(err).Error(), "checksum mismatch") {
		t.Fatalf("Install() error = %v, want checksum mismatch", err)
	}
	if verified {
		t.Fatal("binary verification ran before checksum validation")
	}
}

func TestListSkillsIgnoresRootFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "lark-example"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("metadata"), 0o644); err != nil {
		t.Fatal(err)
	}
	names, err := listSkills(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"lark-example"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

type testZipFile struct {
	content string
	mode    os.FileMode
}

func buildTestZip(t *testing.T, files map[string]testZipFile) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for name, file := range files {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if file.mode != 0 {
			header.SetMode(file.mode)
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(file.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func checksumFor(data []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}
