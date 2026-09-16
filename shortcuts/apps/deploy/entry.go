// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/larksuite/cli/errs"
)

// IndexName is the fixed entry file name inside the published payload. Because
// the entry always ends up as index.html at the payload root, route generation
// keeps working off the existing index.html -> / mapping.
const IndexName = "index.html"

// FallbackAppName is used when no meaningful name can be derived.
const FallbackAppName = "html-app"

// ValidateEntryFileName checks --entry-file. It must name a file sitting
// directly under --dir, so path separators are rejected outright. This
// deliberately does not go through SafeInputPath: that resolves relative to the
// cwd, which conflicts with "directly under --dir". The caller joins the
// validated name onto the already-resolved directory path.
func ValidateEntryFileName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--entry-file must not be empty").WithParam("--entry-file")
	}
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--entry-file must not contain control characters").WithParam("--entry-file")
	}
	if strings.ContainsAny(name, `/\`) {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"--entry-file %q must be a file name directly under --dir, not a path", name).
			WithParam("--entry-file").
			WithHint("subdirectory entries are not supported; point --dir at the subdirectory instead")
	}
	if !strings.EqualFold(filepath.Ext(name), ".html") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--entry-file %q must be an .html file", name).WithParam("--entry-file")
	}
	return nil
}

// ResolveEntry picks the entry file among the names sitting at the payload
// root, covering the four --entry-file / index.html combinations.
func ResolveEntry(entryFlag string, rootNames []string) (string, error) {
	hasIndex, hasEntry := false, false
	for _, n := range rootNames {
		if n == IndexName {
			hasIndex = true
		}
		if entryFlag != "" && n == entryFlag {
			hasEntry = true
		}
	}
	if entryFlag == "" {
		if !hasIndex {
			return "", errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"no entry file: --dir has no %s at its root", IndexName).
				WithParam("--entry-file").
				WithHint("name the entry index.html, or pass --entry-file <name.html>")
		}
		return IndexName, nil
	}
	if !hasEntry {
		return "", errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"--entry-file %q not found directly under --dir", entryFlag).WithParam("--entry-file")
	}
	if hasIndex {
		return "", errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"entry conflict: --entry-file %q and the existing %s would both become the published entry",
			entryFlag, IndexName).
			WithParam("--entry-file").
			WithHint("drop --entry-file to publish the existing index.html, or move index.html out of --dir")
	}
	return entryFlag, nil
}

// DeriveAppName derives the name used when auto-creating an app: the entry file
// name without its extension, falling back to the parent directory when the
// entry is index (which carries no information).
func DeriveAppName(absEntry string) string {
	base := strings.TrimSuffix(filepath.Base(absEntry), filepath.Ext(absEntry))
	if base != "" && !strings.EqualFold(base, "index") {
		return base
	}
	parent := filepath.Base(filepath.Dir(absEntry))
	if parent != "" && parent != "." && parent != string(filepath.Separator) {
		return parent
	}
	return FallbackAppName
}
