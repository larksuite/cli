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
	"runtime"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/internal/skillscheck"
	"github.com/larksuite/cli/internal/vfs"
)

func TestInstallPreparedRejectsConcurrentUpdate(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := vfs.MkdirAll(core.GetBaseConfigDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	lock := lockfile.New(filepath.Join(core.GetBaseConfigDir(), "distribution-update.lock"))
	if err := lock.TryLock(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Unlock() })

	err := installPrepared(&preparedUpdate{Manifest: &Manifest{}}, InstallOptions{})
	if !errors.Is(err, lockfile.ErrHeld) {
		t.Fatalf("installPrepared() error = %v, want lock held", err)
	}

	typed := installError("failed to install distribution update", err)
	problem, ok := errs.ProblemOf(typed)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("lock contention problem = %#v, want validation/failed_precondition", problem)
	}
	if strings.Contains(problem.Hint, "--force") {
		t.Fatalf("lock contention hint = %q, want a retry-later hint", problem.Hint)
	}
}

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

func TestInstallPreparedVerificationFailureDoesNotMutate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	executable := filepath.Join(root, "bin", "lark-cli")
	mustWrite(t, executable, "old")
	binary := filepath.Join(root, "prepared", "lark-cli")
	mustWrite(t, binary, "new")
	mustWrite(t, filepath.Join(root, "prepared", "skills", "managed", "SKILL.md"), "new")
	prepared := &preparedUpdate{
		Manifest:   &Manifest{Version: "target"},
		BinaryPath: binary,
		SkillsRoot: filepath.Join(root, "prepared", "skills"),
	}
	err := installPrepared(prepared, InstallOptions{
		ExecutablePath: executable,
		SkillsDir:      filepath.Join(root, "skills"),
		VerifyBinary:   func(string, string) error { return errors.New("bad binary") },
	})
	if err == nil {
		t.Fatal("installPrepared succeeded")
	}
	assertFile(t, executable, "old")
}

func TestInstallPreparedBinaryCommitFailureRollsBackSkillsAndState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	executable := filepath.Join(root, "bin", "lark-cli")
	// Force the binary commit to fail on either platform contract: on Unix the
	// single atomic rename rejects a file-over-directory target; on Windows the
	// two-phase replace first removes the stale .old backup, which rejects a
	// non-empty directory.
	keepPath := executable
	if runtime.GOOS == "windows" {
		mustWrite(t, filepath.Join(executable+".old", "block-removal"), "blocked")
		mustWrite(t, executable, "old")
	} else {
		keepPath = filepath.Join(executable, "keep")
		mustWrite(t, keepPath, "old")
	}
	skillsDir := filepath.Join(root, "skills")
	mustWrite(t, filepath.Join(skillsDir, "managed", "SKILL.md"), "old")
	if err := skillscheck.WriteState(skillscheck.SkillsState{Version: "old", OfficialSkills: []string{"managed"}}); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "prepared", "lark-cli")
	mustWrite(t, binary, "new")
	mustWrite(t, filepath.Join(root, "prepared", "skills", "managed", "SKILL.md"), "new")
	prepared := &preparedUpdate{
		Manifest:   &Manifest{Version: "target"},
		BinaryPath: binary,
		SkillsRoot: filepath.Join(root, "prepared", "skills"),
	}
	if err := installPrepared(prepared, InstallOptions{
		ExecutablePath: executable,
		SkillsDir:      skillsDir,
		VerifyBinary:   func(string, string) error { return nil },
	}); err == nil {
		t.Fatal("installPrepared succeeded")
	}
	assertFile(t, filepath.Join(skillsDir, "managed", "SKILL.md"), "old")
	state, ok, err := skillscheck.ReadState()
	if err != nil || !ok || state.Version != "old" {
		t.Fatalf("state after rollback = %#v, %v, %v", state, ok, err)
	}
	assertFile(t, keepPath, "old")
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

func mustWrite(t *testing.T, path, value string) {
	t.Helper()
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := vfs.WriteFile(path, []byte(value), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := vfs.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
