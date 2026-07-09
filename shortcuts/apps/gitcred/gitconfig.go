// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package gitcred

import (
	"context"
	"os/exec"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
)

type GitConfig interface {
	SetHelper(ctx context.Context, gitHTTPURL, appID string) error
	UnsetHelper(ctx context.Context, gitHTTPURL string) error
}

type GlobalGitConfig struct {
	HelperCommand string
}

func (g GlobalGitConfig) SetHelper(ctx context.Context, gitHTTPURL, appID string) error {
	normalizedURL, err := NormalizeGitHTTPURL(gitHTTPURL)
	if err != nil {
		return err
	}
	appID = strings.TrimSpace(appID)
	if err := validate.ResourceName(appID, "appID"); err != nil {
		return err
	}
	helper := g.helperCommand(appID)
	helperKey := gitCredentialKey(normalizedURL, "helper")
	useHTTPPathKey := gitCredentialKey(normalizedURL, "useHttpPath")
	previousHelper, hadHelper, err := gitConfigGet(ctx, helperKey)
	if err != nil {
		return err
	}
	if hadHelper && previousHelper != helper && !g.isManagedHelper(previousHelper) {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "git credential helper already configured for %s; refusing to overwrite non-lark helper", normalizedURL)
	}
	if err := exec.CommandContext(ctx, "git", "config", "--global", helperKey, helper).Run(); err != nil {
		return err
	}
	if err := exec.CommandContext(ctx, "git", "config", "--global", useHTTPPathKey, "true").Run(); err != nil {
		if !hadHelper {
			_ = exec.CommandContext(ctx, "git", "config", "--global", "--unset", helperKey).Run()
		} else if previousHelper != helper {
			_ = exec.CommandContext(ctx, "git", "config", "--global", helperKey, previousHelper).Run()
		}
		return err
	}
	return nil
}

func (g GlobalGitConfig) UnsetHelper(ctx context.Context, gitHTTPURL string) error {
	normalizedURL, err := NormalizeGitHTTPURL(gitHTTPURL)
	if err != nil {
		return err
	}
	helperKey := gitCredentialKey(normalizedURL, "helper")
	useHTTPPathKey := gitCredentialKey(normalizedURL, "useHttpPath")
	helper, found, err := gitConfigGet(ctx, helperKey)
	if err != nil {
		return err
	}
	if found {
		if !g.isManagedHelper(helper) {
			return nil
		}
	}
	if err := gitConfigUnset(ctx, helperKey); err != nil {
		return err
	}
	if err := gitConfigUnset(ctx, useHTTPPathKey); err != nil {
		return err
	}
	return nil
}

func (g GlobalGitConfig) helperCommand(appID string) string {
	if g.HelperCommand != "" {
		return g.HelperCommand
	}
	return "!lark-cli apps git-credential-helper --app-id " + shellQuoteArg(appID)
}

func (g GlobalGitConfig) isManagedHelper(helper string) bool {
	helper = strings.TrimSpace(helper)
	if g.HelperCommand != "" {
		return helper == g.HelperCommand
	}
	return strings.HasPrefix(helper, "!lark-cli apps git-credential-helper ")
}

func gitCredentialKey(gitHTTPURL, name string) string {
	return "credential." + gitHTTPURL + "." + name
}

func gitConfigGet(ctx context.Context, key string) (string, bool, error) {
	out, err := exec.CommandContext(ctx, "git", "config", "--global", "--get", key).Output()
	if err == nil {
		return strings.TrimSpace(string(out)), true, nil
	}
	if isGitConfigGetMissing(err) {
		return "", false, nil
	}
	return "", false, errs.NewInternalError(errs.SubtypeExternalTool, "git config get %s failed: %v", key, err).WithCause(err)
}

func gitConfigUnset(ctx context.Context, key string) error {
	if err := exec.CommandContext(ctx, "git", "config", "--global", "--unset", key).Run(); err != nil {
		if isGitConfigUnsetMissing(err) {
			return nil
		}
		return errs.NewInternalError(errs.SubtypeExternalTool, "git config unset %s failed: %v", key, err).WithCause(err)
	}
	return nil
}

func isGitConfigGetMissing(err error) bool {
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return true
	}
	return false
}

func isGitConfigUnsetMissing(err error) bool {
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 5 {
		return true
	}
	return false
}

func shellQuoteArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
