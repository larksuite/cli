// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/shortcuts/apps/deploy"
	"github.com/larksuite/cli/shortcuts/common"
)

// isHTMLDeployMode reports whether +deploy should take the bare-HTML path.
// With neither flag set the existing spark.json project mode runs unchanged.
func isHTMLDeployMode(filePath, dir string) bool {
	return strings.TrimSpace(filePath) != "" || strings.TrimSpace(dir) != ""
}

// validateHTMLDeployFlags checks the flag combination for the bare-HTML path.
func validateHTMLDeployFlags(filePath, dir, entryFile string, skipBuild, noVerify bool) error {
	filePath, dir, entryFile = strings.TrimSpace(filePath), strings.TrimSpace(dir), strings.TrimSpace(entryFile)
	if filePath != "" && dir != "" {
		return appsValidationParamError("--dir",
			"--file-path and --dir are mutually exclusive: use --file-path for a single HTML file, --dir for a directory")
	}
	if entryFile != "" && dir == "" {
		return appsValidationParamError("--entry-file", "--entry-file only applies together with --dir")
	}
	if skipBuild {
		return appsValidationParamError("--skip-build", "--skip-build only applies to the spark.json project mode")
	}
	if noVerify {
		return appsValidationParamError("--no-verify", "--no-verify only applies to the spark.json project mode")
	}
	if filePath != "" && !strings.EqualFold(filepath.Ext(filePath), ".html") {
		return appsValidationParamError("--file-path", "--file-path %q must point at an .html file", filePath)
	}
	if entryFile != "" {
		return deploy.ValidateEntryFileName(entryFile)
	}
	return nil
}

// validateHTMLDeploy is the bare-HTML branch of AppsDeploy.Validate. It checks
// the flag combination, collects the payload off disk and runs the credential
// scan plus the pre-pack size caps. The guard deliberately runs here rather
// than in DryRun so that --dry-run also exits non-zero on a hit.
func validateHTMLDeploy(rctx *common.RuntimeContext) error {
	filePath := strings.TrimSpace(rctx.Str("file-path"))
	dir := strings.TrimSpace(rctx.Str("dir"))
	entryFile := strings.TrimSpace(rctx.Str("entry-file"))
	if err := validateHTMLDeployFlags(filePath, dir, entryFile, rctx.Bool("skip-build"), rctx.Bool("no-verify")); err != nil {
		return err
	}

	var candidates []deploy.Candidate
	if filePath != "" {
		var err error
		candidates, _, err = deploy.CollectFile(rctx.FileIO(), filePath)
		if err != nil {
			return err
		}
	} else {
		cands, rootNames, _, err := deploy.CollectDir(rctx.FileIO(), dir)
		if err != nil {
			return err
		}
		if _, err := deploy.ResolveEntry(entryFile, rootNames); err != nil {
			return err
		}
		candidates = cands
	}

	if _, err := deploy.Guard(candidates, rctx.Bool("allow-sensitive"), deploy.DefaultLimits()); err != nil {
		return err
	}
	return nil
}
