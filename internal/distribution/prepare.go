// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/vfs"
)

// preparedUpdate contains fully downloaded, checksum-verified, extracted
// resources owned by one Install call.
type preparedUpdate struct {
	Manifest   *Manifest
	BinaryPath string
	SkillsRoot string
	root       string
}

// prepareUpdate downloads and validates every resource before installed state
// is mutated.
func prepareUpdate(ctx context.Context, manifest *Manifest) (*preparedUpdate, error) {
	if manifest == nil {
		return nil, fmt.Errorf("distribution manifest is nil")
	}
	if err := vfs.MkdirAll(core.GetBaseConfigDir(), 0o700); err != nil {
		return nil, err
	}
	root, err := vfs.MkdirTemp(core.GetBaseConfigDir(), ".distribution-update-*")
	if err != nil {
		return nil, err
	}
	prepared := &preparedUpdate{Manifest: manifest, root: root}
	keep := false
	defer func() {
		if !keep {
			prepared.cleanup()
		}
	}()

	binaryRoot, err := prepareArtifact(ctx, manifest, CurrentPlatformKey(), root, "binary")
	if err != nil {
		return nil, err
	}
	executableName := "lark-cli"
	if runtime.GOOS == "windows" {
		executableName += ".exe"
	}
	prepared.BinaryPath = filepath.Join(binaryRoot, executableName)
	info, err := vfs.Stat(prepared.BinaryPath)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("binary artifact must contain %s at its root", executableName)
	}
	prepared.SkillsRoot, err = prepareArtifact(ctx, manifest, SkillsKey, root, "skills")
	if err != nil {
		return nil, err
	}
	keep = true
	return prepared, nil
}

func prepareArtifact(ctx context.Context, manifest *Manifest, key, root, directory string) (string, error) {
	archive, err := downloadArtifact(ctx, manifest.Artifacts[key], root, directory+"-*.archive")
	if err != nil {
		return "", fmt.Errorf("download %s artifact: %w", key, err)
	}
	destination := filepath.Join(root, directory)
	if err := vfs.MkdirAll(destination, 0o700); err != nil {
		return "", err
	}
	if err := extractArchive(archive, destination); err != nil {
		return "", fmt.Errorf("extract %s artifact: %w", key, err)
	}
	return destination, nil
}

// cleanup removes downloaded and extracted temporary resources.
func (p *preparedUpdate) cleanup() {
	if p != nil && p.root != "" {
		_ = vfs.RemoveAll(p.root)
	}
}
