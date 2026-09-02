// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/internal/selfupdate"
	"github.com/larksuite/cli/internal/skillscheck"
	"github.com/larksuite/cli/internal/vfs"
)

// InstallOptions supplies destinations and test seams for a distribution update.
type InstallOptions struct {
	ExecutablePath string
	// SkillsDir overrides automatic Agent directory discovery when non-empty.
	SkillsDir    string
	VerifyBinary func(path, version string) error
}

// Install downloads, verifies, and commits the configured Skills and binary
// resources as one rollback-capable local transaction. The executable is
// committed last.
func Install(ctx context.Context, manifest *Manifest, opts InstallOptions) errs.TypedError {
	prepared, typedErr := prepareUpdate(ctx, manifest)
	if typedErr != nil {
		return typedErr
	}
	defer prepared.cleanup()
	if err := installPrepared(prepared, opts); err != nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "failed to install distribution update: %s", err).
			WithHint("Retry with `lark-cli update --force`.").
			WithCause(err)
	}
	return nil
}

// SyncSkills repairs the managed Skills from a manifest without replacing an
// already-matching binary.
func SyncSkills(ctx context.Context, manifest *Manifest, opts InstallOptions) errs.TypedError {
	if manifest == nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "distribution manifest is nil")
	}
	if err := vfs.MkdirAll(core.GetBaseConfigDir(), 0o700); err != nil {
		return prepareFileError(err)
	}
	root, err := vfs.MkdirTemp(core.GetBaseConfigDir(), ".distribution-skills-*")
	if err != nil {
		return prepareFileError(err)
	}
	defer func() { _ = vfs.RemoveAll(root) }()

	skillsRoot, typedErr := prepareArtifact(ctx, manifest, SkillsKey, root, "skills")
	if typedErr != nil {
		return typedErr
	}
	if err := withInstallLock(func() error {
		_, finalize, err := syncPreparedSkills(skillsRoot, manifest, opts.SkillsDir)
		if err == nil {
			finalize()
		}
		return err
	}); err != nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "failed to synchronize distribution Skills: %s", err).
			WithHint("Retry with `lark-cli update --force`.").
			WithCause(err)
	}
	return nil
}

func installPrepared(prepared *preparedUpdate, opts InstallOptions) error {
	if prepared == nil || prepared.Manifest == nil {
		return fmt.Errorf("prepared distribution update is required")
	}
	return withInstallLock(func() error { return installPreparedLocked(prepared, opts) })
}

func installPreparedLocked(prepared *preparedUpdate, opts InstallOptions) error {
	candidate, err := selfupdate.PrepareCandidate(
		prepared.BinaryPath,
		opts.ExecutablePath,
		prepared.Manifest.Version,
		opts.VerifyBinary,
	)
	if err != nil {
		return fmt.Errorf("prepare binary: %w", err)
	}
	defer candidate.Cleanup()

	rollbackSkills, finalizeSkills, err := syncPreparedSkills(
		prepared.SkillsRoot,
		prepared.Manifest,
		opts.SkillsDir,
	)
	if err != nil {
		return err
	}
	finalizeBinary, err := candidate.Install()
	if err != nil {
		cause := fmt.Errorf("replace binary: %w", err)
		if rollbackErr := rollbackSkills(); rollbackErr != nil {
			return fmt.Errorf("%w (Skills rollback failed: %w)", cause, rollbackErr)
		}
		return cause
	}
	finalizeSkills()
	finalizeBinary()
	return nil
}

func syncPreparedSkills(root string, manifest *Manifest, targetDir string) (func() error, func(), error) {
	return skillscheck.SyncPreparedTree(skillscheck.PreparedTreeOptions{
		Root:           root,
		Version:        manifest.Version,
		SourceIdentity: manifest.sourceIdentity,
		TargetDir:      targetDir,
	})
}

func withInstallLock(fn func() error) error {
	if err := vfs.MkdirAll(core.GetBaseConfigDir(), 0o700); err != nil {
		return err
	}
	lock := lockfile.New(filepath.Join(core.GetBaseConfigDir(), "distribution-update.lock"))
	if err := lock.TryLock(); err != nil {
		return fmt.Errorf("acquire distribution update lock: %w", err)
	}
	defer func() { _ = lock.Unlock() }()
	return fn()
}

// preparedUpdate contains fully downloaded, checksum-verified, extracted
// resources owned by one Install call.
type preparedUpdate struct {
	Manifest   *Manifest
	BinaryPath string
	SkillsRoot string
	root       string
}

// cleanup removes downloaded and extracted temporary resources.
func (p *preparedUpdate) cleanup() {
	if p != nil && p.root != "" {
		_ = vfs.RemoveAll(p.root)
	}
}

// prepareUpdate downloads and validates every resource before installed state
// is mutated.
func prepareUpdate(ctx context.Context, manifest *Manifest) (*preparedUpdate, errs.TypedError) {
	if manifest == nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "distribution manifest is nil")
	}
	if err := vfs.MkdirAll(core.GetBaseConfigDir(), 0o700); err != nil {
		return nil, prepareFileError(err)
	}
	root, err := vfs.MkdirTemp(core.GetBaseConfigDir(), ".distribution-update-*")
	if err != nil {
		return nil, prepareFileError(err)
	}
	prepared := &preparedUpdate{Manifest: manifest, root: root}
	keep := false
	defer func() {
		if !keep {
			prepared.cleanup()
		}
	}()

	binaryRoot, typedErr := prepareArtifact(ctx, manifest, CurrentPlatformKey(), root, "binary")
	if typedErr != nil {
		return nil, typedErr
	}
	executableName := "lark-cli"
	if runtime.GOOS == "windows" {
		executableName += ".exe"
	}
	prepared.BinaryPath = filepath.Join(binaryRoot, executableName)
	info, err := vfs.Stat(prepared.BinaryPath)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"binary artifact must contain %s at its root", executableName)
	}
	prepared.SkillsRoot, typedErr = prepareArtifact(ctx, manifest, SkillsKey, root, "skills")
	if typedErr != nil {
		return nil, typedErr
	}
	keep = true
	return prepared, nil
}

func prepareArtifact(ctx context.Context, manifest *Manifest, key, root, directory string) (string, errs.TypedError) {
	archive, err := downloadArtifact(ctx, manifest.Artifacts[key], root, directory+"-*.archive")
	if err != nil {
		return "", classifyArtifactError("download", key, err)
	}
	destination := filepath.Join(root, directory)
	if err := vfs.MkdirAll(destination, 0o700); err != nil {
		return "", prepareFileError(err)
	}
	if err := extractArchive(archive, destination); err != nil {
		return "", classifyArtifactError("extract", key, err)
	}
	return destination, nil
}

// classifyArtifactError attributes an artifact-stage failure: local file I/O
// is FileIO; fetch, size-limit, checksum, and archive-format failures mean the
// delivered artifact is missing or broken and are reported as network/protocol.
func classifyArtifactError(stage, key string, err error) errs.TypedError {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return prepareFileError(err)
	}
	return errs.NewNetworkError(errs.SubtypeNetworkProtocol, "failed to %s %s artifact: %s", stage, key, err).
		WithCause(err)
}

func prepareFileError(err error) errs.TypedError {
	return errs.NewInternalError(errs.SubtypeFileIO, "failed to prepare distribution update: %s", err).
		WithCause(err)
}
