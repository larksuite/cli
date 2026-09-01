// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

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
	binaryArchive := buildTestZip(t, map[string]string{executableName: "new binary"})
	skillsArchive := buildTestZip(t, map[string]string{
		"README.md":           "bundle metadata",
		"lark-alpha/SKILL.md": "alpha",
		"lark-beta/SKILL.md":  "beta",
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
		Version: "release-channel-7",
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
	assertFile(t, filepath.Join(skillsDir, "lark-beta", "SKILL.md"), "beta")
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

func buildTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
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
