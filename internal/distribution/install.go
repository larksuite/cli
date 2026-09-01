// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
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
	executable, skillsDirs, err := resolveInstallDestinations(opts)
	if err != nil {
		return err
	}
	if opts.VerifyBinary == nil {
		opts.VerifyBinary = verifyBinaryVersion
	}

	stagedBinary, err := stageBinary(prepared.BinaryPath, executable)
	if err != nil {
		return fmt.Errorf("stage binary: %w", err)
	}
	defer func() { _ = vfs.Remove(stagedBinary) }()
	if err := opts.VerifyBinary(stagedBinary, prepared.Manifest.Version); err != nil {
		return fmt.Errorf("verify staged binary: %w", err)
	}

	previous, _, err := skillscheck.ReadState()
	if err != nil {
		return fmt.Errorf("read Skills state: %w", err)
	}
	restoreState, err := skillscheck.SnapshotState()
	if err != nil {
		return fmt.Errorf("snapshot Skills state: %w", err)
	}
	rollbackSkills, finalizeSkills, err := installSkillsToTargets(prepared, skillsDirs, previous)
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		var failures []string
		if err := rollbackSkills(); err != nil {
			failures = append(failures, "Skills: "+err.Error())
		}
		if err := restoreState(); err != nil {
			failures = append(failures, "state: "+err.Error())
		}
		if len(failures) > 0 {
			return fmt.Errorf("%w (rollback failed: %s)", cause, strings.Join(failures, "; "))
		}
		return cause
	}

	state := skillscheck.NewCompleteState(
		prepared.Manifest.Version,
		skillscheck.LayoutSeparate,
		prepared.SkillNames,
		previous,
	)
	state.SourceIdentity = prepared.Manifest.sourceIdentity
	if err := skillscheck.WriteState(state); err != nil {
		return rollback(fmt.Errorf("write Skills state: %w", err))
	}

	finalizeBinary, err := replaceBinary(stagedBinary, executable)
	if err != nil {
		return rollback(fmt.Errorf("replace binary: %w", err))
	}
	finalizeSkills()
	finalizeBinary()
	return nil
}
