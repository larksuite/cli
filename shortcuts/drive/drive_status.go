// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

type driveStatusEntry struct {
	RelPath   string `json:"rel_path"`
	FileToken string `json:"file_token,omitempty"`
}

type driveStatusLocalFile struct {
	PathToCwd string
	ModTime   time.Time
}

type driveStatusRemoteFile struct {
	FileToken    string
	ModifiedTime string
}

const (
	driveStatusDetectionExact = "exact"
	driveStatusDetectionQuick = "quick"
)

// DriveStatus walks --local-dir, recursively lists --folder-token, and reports
// four buckets (new_local, new_remote, modified, unchanged) either by exact
// SHA-256 hash (default) or by a quick modified_time comparison (--quick).
//
// Only Drive entries with type=file are compared; online docs (docx, sheet,
// bitable, mindnote, slides) and shortcuts are skipped because there is no
// equivalent local binary to hash against.
//
// SafeInputPath (applied by runtime.FileIO()) rejects absolute paths and any
// path that resolves outside cwd, which keeps the local side bounded to the
// caller's working directory.
var DriveStatus = common.Shortcut{
	Service:           "drive",
	Command:           "+status",
	Description:       "Compare a local directory with a Drive folder by exact hash or quick modified_time",
	Risk:              "read",
	Scopes:            []string{"drive:drive.metadata:readonly"},
	ConditionalScopes: []string{"drive:file:download"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "local-dir", Desc: "local root directory (relative to cwd)", Required: true},
		{Name: "folder-token", Desc: "Drive folder token", Required: true},
		{Name: "quick", Type: "bool", Desc: "compare modified_time only and skip remote downloads for files present on both sides"},
	},
	Tips: []string{
		"Only entries with type=file are compared; online docs (docx, sheet, bitable, mindnote, slides) and shortcuts are skipped.",
		"Default detection=exact downloads files present on both sides and SHA-256 hashes them in memory; expect noticeable I/O on large folders.",
		"Pass --quick for the recommended fast preflight mode: it compares local mtime with Drive modified_time, skips remote downloads, and reports detection=quick as a best-effort diff.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		localDir := strings.TrimSpace(runtime.Str("local-dir"))
		folderToken := strings.TrimSpace(runtime.Str("folder-token"))
		if localDir == "" {
			return common.FlagErrorf("--local-dir is required")
		}
		if folderToken == "" {
			return common.FlagErrorf("--folder-token is required")
		}
		if err := validate.ResourceName(folderToken, "--folder-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
		// Path safety (absolute paths, traversal, symlink escape) is enforced
		// upfront by the framework helper so the error message references the
		// correct flag name; FileIO().Stat below would do the same check, but
		// surface --file in its hint.
		if _, err := validate.SafeLocalFlagPath("--local-dir", localDir); err != nil {
			return output.ErrValidation("%s", err)
		}
		info, err := runtime.FileIO().Stat(localDir)
		if err != nil {
			return common.WrapInputStatError(err)
		}
		if !info.IsDir() {
			return output.ErrValidation("--local-dir is not a directory: %s", localDir)
		}
		// Conditional scope pre-check: quick mode only compares local mtime with
		// Drive modified_time, so it must not be blocked on the download grant.
		// Exact mode hashes remote bytes, which requires drive:file:download. Do
		// the stricter check here once we know which execution path the flags
		// selected. EnsureScopes is a silent no-op when scope metadata is
		// unavailable, so environments without token scope introspection still
		// proceed and rely on the API-level missing_scope error if needed.
		if !runtime.Bool("quick") {
			if err := runtime.EnsureScopes([]string{"drive:file:download"}); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		desc := "Walk --local-dir, recursively list --folder-token, and download files present on both sides to compare SHA-256."
		if runtime.Bool("quick") {
			desc = "Walk --local-dir, recursively list --folder-token, and compare local mtime with Drive modified_time for files present on both sides without downloading remote bytes."
		}
		return common.NewDryRunAPI().
			Desc(desc).
			GET("/open-apis/drive/v1/files").
			Set("folder_token", runtime.Str("folder-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		localDir := strings.TrimSpace(runtime.Str("local-dir"))
		folderToken := strings.TrimSpace(runtime.Str("folder-token"))
		detection := driveStatusDetectionExact
		if runtime.Bool("quick") {
			detection = driveStatusDetectionQuick
		}

		// Resolve --local-dir to its canonical absolute path before walking.
		// SafeInputPath fully evaluates symlinks across the entire path,
		// which closes the kernel-level escape route that filepath.Clean
		// alone misses: e.g. "link/.." string-cleans to "." but the kernel
		// resolves through link's target's parent, so a raw walk on the
		// user-supplied string can land outside cwd. Walking the canonical
		// root sidesteps that — and the matching cwd canonical lets each
		// absolute walk hit be converted to a cwd-relative path that
		// FileIO.Open's SafeInputPath check still accepts.
		//
		// Validate already ran SafeLocalFlagPath (with the proper flag
		// name in the error message), so a failure here is unexpected and
		// only possible under a Validate↔Execute race.
		safeRoot, err := validate.SafeInputPath(localDir)
		if err != nil {
			return output.ErrValidation("--local-dir: %s", err)
		}
		cwdCanonical, err := validate.SafeInputPath(".")
		if err != nil {
			return output.ErrValidation("could not resolve cwd: %s", err)
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Walking local: %s\n", localDir)
		localFiles, err := walkLocalForStatus(safeRoot, cwdCanonical)
		if err != nil {
			return err
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Listing Drive folder: %s\n", common.MaskToken(folderToken))
		entries, err := listRemoteFolderEntries(ctx, runtime, folderToken, "")
		if err != nil {
			return err
		}
		if duplicates := duplicateRemoteFilePaths(entries); len(duplicates) > 0 {
			return duplicateRemotePathError(duplicates)
		}
		// +status only diffs binary content, so collapse the unified
		// listing to type=file. Online docs / shortcuts have no
		// hashable bytes and are intentionally absent from the diff
		// view (a docx living next to a same-named local file is a
		// known no-op).
		remoteFiles := make(map[string]driveStatusRemoteFile, len(entries))
		for _, entry := range entries {
			if entry.Type == driveTypeFile {
				remoteFiles[entry.RelPath] = driveStatusRemoteFile{FileToken: entry.FileToken, ModifiedTime: entry.ModifiedTime}
			}
		}

		paths := mergeStatusPaths(localFiles, remoteFiles)

		var newLocal, newRemote, modified, unchanged []driveStatusEntry
		for _, relPath := range paths {
			localFile, hasLocal := localFiles[relPath]
			remoteFile, hasRemote := remoteFiles[relPath]
			switch {
			case hasLocal && !hasRemote:
				newLocal = append(newLocal, driveStatusEntry{RelPath: relPath})
			case !hasLocal && hasRemote:
				newRemote = append(newRemote, driveStatusEntry{RelPath: relPath, FileToken: remoteFile.FileToken})
			default:
				entry := driveStatusEntry{RelPath: relPath, FileToken: remoteFile.FileToken}
				if detection == driveStatusDetectionQuick {
					if driveStatusShouldTreatAsUnchangedQuick(remoteFile.ModifiedTime, localFile.ModTime) {
						unchanged = append(unchanged, entry)
					} else {
						modified = append(modified, entry)
					}
					continue
				}
				localHash, err := hashLocalForStatus(runtime, localFile.PathToCwd)
				if err != nil {
					return err
				}
				remoteHash, err := hashRemoteForStatus(ctx, runtime, remoteFile.FileToken)
				if err != nil {
					return err
				}
				if localHash == remoteHash {
					unchanged = append(unchanged, entry)
				} else {
					modified = append(modified, entry)
				}
			}
		}

		runtime.Out(map[string]interface{}{
			"detection":  detection,
			"new_local":  emptyIfNil(newLocal),
			"new_remote": emptyIfNil(newRemote),
			"modified":   emptyIfNil(modified),
			"unchanged":  emptyIfNil(unchanged),
		}, nil)
		return nil
	},
}

// walkLocalForStatus walks the canonical absolute root produced by
// SafeInputPath. Using the canonical root keeps the kernel from
// following any symlink hidden inside the user-supplied --local-dir
// (e.g. "link/..", which filepath.Clean shrinks to "." but which OS
// path resolution would resolve through the symlink target). For each
// hit, we report rel_path relative to root for the JSON output, and
// convert the absolute path to a cwd-relative form so FileIO.Open's
// SafeInputPath check (which rejects absolute paths) still applies.
func walkLocalForStatus(root, cwdCanonical string) (map[string]driveStatusLocalFile, error) {
	files := make(map[string]driveStatusLocalFile)
	// FileIO has no walker today and shortcuts can't import internal/vfs.
	// The walk root is the canonical absolute path returned by
	// validate.SafeInputPath, so it is no longer a symlink itself, and
	// WalkDir's default policy (do not follow child symlinks) keeps the
	// traversal inside that canonical subtree.
	err := filepath.WalkDir(root, func(absPath string, d fs.DirEntry, walkErr error) error { //nolint:forbidigo // see comment above
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, absPath)
		if err != nil {
			return err
		}
		relToCwd, err := filepath.Rel(cwdCanonical, absPath)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = driveStatusLocalFile{PathToCwd: relToCwd, ModTime: info.ModTime()}
		return nil
	})
	if err != nil {
		return nil, output.Errorf(output.ExitInternal, "io", "walk %s: %s", root, err)
	}
	return files, nil
}

func driveStatusShouldTreatAsUnchangedQuick(remoteModified string, local time.Time) bool {
	cmp, ok := compareDriveRemoteModifiedToLocal(remoteModified, local)
	return ok && cmp == 0
}

func hashLocalForStatus(runtime *common.RuntimeContext, path string) (string, error) {
	f, err := runtime.FileIO().Open(path)
	if err != nil {
		return "", common.WrapInputStatError(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", output.Errorf(output.ExitInternal, "io", "hash %s: %s", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashRemoteForStatus(ctx context.Context, runtime *common.RuntimeContext, fileToken string) (string, error) {
	resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
		HttpMethod: "GET",
		ApiPath:    fmt.Sprintf("/open-apis/drive/v1/files/%s/download", validate.EncodePathSegment(fileToken)),
	})
	if err != nil {
		return "", output.ErrNetwork("download %s: %s", common.MaskToken(fileToken), err)
	}
	defer resp.Body.Close()
	h := sha256.New()
	if _, err := io.Copy(h, resp.Body); err != nil {
		return "", output.ErrNetwork("hash remote %s: %s", common.MaskToken(fileToken), err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func mergeStatusPaths(local map[string]driveStatusLocalFile, remote map[string]driveStatusRemoteFile) []string {
	seen := make(map[string]struct{}, len(local)+len(remote))
	for p := range local {
		seen[p] = struct{}{}
	}
	for p := range remote {
		seen[p] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func emptyIfNil(s []driveStatusEntry) []driveStatusEntry {
	if s == nil {
		return []driveStatusEntry{}
	}
	return s
}
