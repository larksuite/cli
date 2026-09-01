// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const candidateVerifyTimeout = 10 * time.Second

// CandidateVerifier validates a staged executable before installation.
type CandidateVerifier func(path, version string) error

// Candidate is a verified executable staged beside its target.
type Candidate struct {
	path   string
	target string
}

// PrepareCandidate stages and verifies source without changing the installed
// executable. An empty target selects the current executable.
func PrepareCandidate(source, target, version string, verify CandidateVerifier) (*Candidate, error) {
	resolved, err := resolveCandidateTarget(target)
	if err != nil {
		return nil, err
	}
	if err := vfs.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return nil, err
	}
	in, err := vfs.Open(source)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	path := resolved + ".new"
	keep := false
	defer func() {
		if !keep {
			_ = vfs.Remove(path)
		}
	}()
	if _, err := validate.AtomicWriteFromReader(path, in, 0o755); err != nil {
		return nil, err
	}
	if verify == nil {
		verify = VerifyCandidateVersion
	}
	if err := verify(path, version); err != nil {
		return nil, fmt.Errorf("verify staged binary: %w", err)
	}
	keep = true
	return &Candidate{path: path, target: resolved}, nil
}

func resolveCandidateTarget(target string) (string, error) {
	if target != "" {
		return target, nil
	}
	return New().resolveExe()
}

// Cleanup removes a prepared candidate that was not installed.
func (c *Candidate) Cleanup() {
	if c != nil && c.path != "" {
		_ = vfs.Remove(c.path)
	}
}

// Install atomically promotes the prepared candidate. The returned finalize
// function removes the previous executable after the surrounding update commits.
func (c *Candidate) Install() (func(), error) {
	if c == nil || c.path == "" || c.target == "" {
		return nil, fmt.Errorf("prepared binary candidate is required")
	}
	backup := c.target + ".old"
	targetExists, err := candidatePathExists(c.target)
	if err != nil {
		return nil, err
	}
	backupExists, err := candidatePathExists(backup)
	if err != nil {
		return nil, err
	}
	if targetExists && backupExists {
		if err := vfs.Remove(backup); err != nil {
			return nil, fmt.Errorf("remove stale binary backup: %w", err)
		}
		backupExists = false
	}
	if targetExists {
		if err := vfs.Rename(c.target, backup); err != nil {
			return nil, err
		}
		backupExists = true
	}
	if err := vfs.Rename(c.path, c.target); err != nil {
		if targetExists {
			_ = vfs.Rename(backup, c.target)
		}
		return nil, err
	}
	c.path = ""
	return func() {
		if backupExists {
			_ = vfs.Remove(backup)
		}
	}, nil
}

// VerifyCandidateVersion checks the exact opaque version reported by a binary.
func VerifyCandidateVersion(path, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), candidateVerifyTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").CombinedOutput() //nolint:gosec // path is a checksum-verified staged binary.
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("binary verification timed out after %s", candidateVerifyTimeout)
	}
	if err != nil {
		return fmt.Errorf("run --version: %w", err)
	}
	if strings.TrimSpace(string(output)) != "lark-cli version "+version {
		return fmt.Errorf("binary reported %q, want version %q", strings.TrimSpace(string(output)), version)
	}
	return nil
}

func candidatePathExists(path string) (bool, error) {
	if _, err := vfs.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
