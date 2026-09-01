// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/vfs"
)

// preparedUpdate contains fully downloaded, checksum-verified, extracted
// resources owned by one Install call.
type preparedUpdate struct {
	Manifest   *Manifest
	BinaryPath string
	SkillsRoot string
	SkillNames []string
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

	binaryArchive, err := downloadArtifact(ctx, manifest.Artifacts[CurrentPlatformKey()], root, "binary-*.archive")
	if err != nil {
		return nil, fmt.Errorf("download %s artifact: %w", CurrentPlatformKey(), err)
	}
	skillsArchive, err := downloadArtifact(ctx, manifest.Artifacts[SkillsKey], root, "skills-*.archive")
	if err != nil {
		return nil, fmt.Errorf("download skills artifact: %w", err)
	}

	binaryRoot := filepath.Join(root, "binary")
	if err := vfs.MkdirAll(binaryRoot, 0o700); err != nil {
		return nil, err
	}
	if err := extractArchive(binaryArchive, binaryRoot); err != nil {
		return nil, fmt.Errorf("extract binary artifact: %w", err)
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
	prepared.SkillsRoot = filepath.Join(root, "skills")
	if err := vfs.MkdirAll(prepared.SkillsRoot, 0o700); err != nil {
		return nil, err
	}
	if err := extractArchive(skillsArchive, prepared.SkillsRoot); err != nil {
		return nil, fmt.Errorf("extract skills artifact: %w", err)
	}
	prepared.SkillNames, err = listSkills(prepared.SkillsRoot)
	if err != nil {
		return nil, err
	}
	keep = true
	return prepared, nil
}

// cleanup removes downloaded and extracted temporary resources.
func (p *preparedUpdate) cleanup() {
	if p != nil && p.root != "" {
		_ = vfs.RemoveAll(p.root)
	}
}

func listSkills(root string) ([]string, error) {
	entries, err := vfs.ReadDir(root)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("skills artifact contains no Skills")
	}
	sort.Strings(names)
	return names, nil
}
