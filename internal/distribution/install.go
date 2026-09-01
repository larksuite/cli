// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/selfupdate"
	"github.com/larksuite/cli/internal/skillscheck"
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
	prepared, err := prepareUpdate(ctx, manifest)
	if err != nil {
		return classifyError("failed to prepare distribution update", err)
	}
	defer prepared.cleanup()
	if err := installPrepared(prepared, opts); err != nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "failed to install distribution update: %s", err).
			WithHint("Retry with `lark-cli update --force`.").
			WithCause(err)
	}
	return nil
}

func installPrepared(prepared *preparedUpdate, opts InstallOptions) error {
	if prepared == nil || prepared.Manifest == nil {
		return fmt.Errorf("prepared distribution update is required")
	}
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

	rollbackSkills, finalizeSkills, err := skillscheck.SyncPreparedTree(skillscheck.PreparedTreeOptions{
		Root:           prepared.SkillsRoot,
		Version:        prepared.Manifest.Version,
		SourceIdentity: prepared.Manifest.sourceIdentity,
		TargetDir:      opts.SkillsDir,
	})
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
