// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	drivePullIfExistsOverwrite = "overwrite"
	drivePullIfExistsSkip      = "skip"
)

type drivePullItem struct {
	RelPath   string `json:"rel_path"`
	FileToken string `json:"file_token,omitempty"`
	SourceID  string `json:"source_id,omitempty"`
	Action    string `json:"action"`
	Error     string `json:"error,omitempty"`
}

type drivePullTarget struct {
	DownloadToken string
	ItemFileToken string
	ItemSourceID  string
}

// DrivePull performs a one-way file-level mirror from a Drive folder onto
// a local directory: recursively lists --folder-token, downloads each
// type=file entry under --local-dir, and optionally deletes local files
// absent from Drive (--delete-local --yes).
//
// Only Drive entries with type=file participate; online docs (docx, sheet,
// bitable, mindnote, slides) and shortcuts are skipped because there is no
// equivalent local binary to write back. Directories are reproduced when
// remote folders contain downloadable files, but local directories that
// become orphaned after a remote folder is removed are NOT pruned —
// --delete-local only unlinks regular files.
var DrivePull = common.Shortcut{
	Service:     "drive",
	Command:     "+pull",
	Description: "One-way file-level mirror of a Drive folder onto a local directory (Drive → local)",
	Risk:        "write",
	Scopes:      []string{"drive:drive.metadata:readonly", "drive:file:download"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "local-dir", Desc: "local root directory (relative to cwd)", Required: true},
		{Name: "folder-token", Desc: "source Drive folder token", Required: true},
		{Name: "if-exists", Desc: "policy when a local file already exists", Default: drivePullIfExistsOverwrite, Enum: []string{drivePullIfExistsOverwrite, drivePullIfExistsSkip}},
		{Name: "on-duplicate-remote", Desc: "policy when multiple remote Drive entries map to the same rel_path", Default: driveDuplicateRemoteFail, Enum: []string{driveDuplicateRemoteFail, driveDuplicateRemoteRename, driveDuplicateRemoteNewest, driveDuplicateRemoteOldest}},
		{Name: "delete-local", Type: "bool", Desc: "delete local regular files absent from Drive (file-level mirror; empty directories are NOT pruned); requires --yes"},
		{Name: "yes", Type: "bool", Desc: "confirm --delete-local before deleting local files"},
	},
	Tips: []string{
		"Only entries with type=file are downloaded; online docs (docx, sheet, bitable, mindnote, slides) and shortcuts are skipped.",
		"Subfolders recurse and are reproduced as local directories under --local-dir; missing parents are created automatically.",
		"Duplicate remote rel_path conflicts fail by default. Use --on-duplicate-remote=rename to download duplicate files with stable hashed suffixes.",
		"--delete-local requires --yes; without --yes the command is rejected upfront so a stray flag never deletes anything.",
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
		if runtime.Bool("delete-local") && !runtime.Bool("yes") {
			return output.ErrValidation("--delete-local requires --yes (high-risk: deletes local files absent from Drive)")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			Desc("Recursively list --folder-token, download each type=file entry into --local-dir, and (when --delete-local --yes is set) remove local files absent from Drive.").
			GET("/open-apis/drive/v1/files").
			Set("folder_token", runtime.Str("folder-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		localDir := strings.TrimSpace(runtime.Str("local-dir"))
		folderToken := strings.TrimSpace(runtime.Str("folder-token"))
		ifExists := strings.TrimSpace(runtime.Str("if-exists"))
		if ifExists == "" {
			ifExists = drivePullIfExistsOverwrite
		}
		duplicateRemote := strings.TrimSpace(runtime.Str("on-duplicate-remote"))
		if duplicateRemote == "" {
			duplicateRemote = driveDuplicateRemoteFail
		}
		deleteLocal := runtime.Bool("delete-local")

		// Resolve --local-dir to its canonical absolute path before we
		// touch the filesystem. SafeInputPath fully evaluates symlinks
		// across the entire path; this matters because filepath.Clean
		// alone shrinks "link/.." to "." while the kernel resolves it
		// through the symlink target's parent — meaning a raw walk on
		// the user-supplied string can land outside cwd. Walking the
		// canonical root sidesteps that, and using cwd canonical lets
		// us emit cwd-relative download targets that FileIO.Save's
		// SafeOutputPath check still accepts. The risk is much higher
		// here than in +status because --delete-local would otherwise
		// remove the wrong files outside cwd.
		safeRoot, err := validate.SafeInputPath(localDir)
		if err != nil {
			return output.ErrValidation("--local-dir: %s", err)
		}
		cwdCanonical, err := validate.SafeInputPath(".")
		if err != nil {
			return output.ErrValidation("could not resolve cwd: %s", err)
		}
		// rootRelToCwd is the localDir form FileIO.Save accepts (it
		// rejects absolute paths). For cwd itself it becomes ".", which
		// joins cleanly with the rel_paths returned by the lister.
		rootRelToCwd, err := filepath.Rel(cwdCanonical, safeRoot)
		if err != nil {
			return output.ErrValidation("--local-dir resolves outside cwd: %s", err)
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Listing Drive folder: %s\n", common.MaskToken(folderToken))
		entries, err := listRemoteFolderEntries(ctx, runtime, folderToken, "")
		if err != nil {
			return err
		}
		if duplicates := blockingRemotePathConflicts(entries, duplicateRemote); len(duplicates) > 0 {
			return duplicateRemotePathError(duplicates)
		}
		// Two views over the same listing:
		//   - remoteFiles drives the download/skip loop (only type=file
		//     has hashable bytes the local mirror can write back).
		//   - remotePaths is the --delete-local guard: it carries every
		//     rel_path Drive owns regardless of type, so a local file
		//     shadowed by a remote folder / online doc / shortcut is NOT
		//     treated as orphaned.
		remoteFiles, remotePaths, err := drivePullRemoteViews(entries, duplicateRemote)
		if err != nil {
			return output.Errorf(output.ExitInternal, "internal", "%s", err)
		}

		var downloaded, skipped, failed, deletedLocal int
		downloadFailed := 0
		items := make([]drivePullItem, 0)

		// Deterministic iteration order for output stability.
		downloadablePaths := make([]string, 0, len(remoteFiles))
		for p := range remoteFiles {
			downloadablePaths = append(downloadablePaths, p)
		}
		sort.Strings(downloadablePaths)

		for _, rel := range downloadablePaths {
			targetFile := remoteFiles[rel]
			downloadToken := targetFile.DownloadToken
			itemFileToken := targetFile.ItemFileToken
			itemSourceID := targetFile.ItemSourceID
			target := filepath.Join(rootRelToCwd, rel)

			if info, statErr := runtime.FileIO().Stat(target); statErr == nil {
				// Mirror conflict: remote is a regular file but local
				// has a directory at the same rel_path. Neither
				// "skipped" nor "downloaded" describes reality —
				// SafeOutputPath would refuse to write a file over a
				// directory, and pretending the directory is a
				// pre-existing file under --if-exists=skip silently
				// hides the conflict. Surface as a failure.
				if info.IsDir() {
					items = append(items, drivePullItem{
						RelPath:   rel,
						FileToken: itemFileToken,
						SourceID:  itemSourceID,
						Action:    "failed",
						Error:     fmt.Sprintf("local path is a directory, remote is a regular file: %s", target),
					})
					failed++
					downloadFailed++
					continue
				}
				if ifExists == drivePullIfExistsSkip {
					items = append(items, drivePullItem{RelPath: rel, FileToken: itemFileToken, SourceID: itemSourceID, Action: "skipped"})
					skipped++
					continue
				}
			}

			if err := drivePullDownload(ctx, runtime, downloadToken, target); err != nil {
				items = append(items, drivePullItem{RelPath: rel, FileToken: itemFileToken, SourceID: itemSourceID, Action: "failed", Error: err.Error()})
				failed++
				downloadFailed++
				continue
			}
			items = append(items, drivePullItem{RelPath: rel, FileToken: itemFileToken, SourceID: itemSourceID, Action: "downloaded"})
			downloaded++
		}

		// Gate --delete-local on a clean download pass. With download
		// failures still in items[], proceeding to the delete walk would
		// leave the mirror in a half-synced state where some files Drive
		// owns are missing locally AND some local-only files have been
		// removed. Surface the failure first; the operator can re-run
		// after fixing whatever caused the download error.
		if deleteLocal && downloadFailed == 0 {
			// Walk the canonical absolute root, build the list of
			// rel_paths, then delete via the absolute path. Both
			// values come from the validated safeRoot, so kernel
			// path resolution cannot redirect the delete to a file
			// outside the canonical subtree.
			localAbsPaths, err := drivePullWalkLocal(safeRoot)
			if err != nil {
				return err
			}
			for _, absPath := range localAbsPaths {
				rel, relErr := filepath.Rel(safeRoot, absPath)
				if relErr != nil {
					items = append(items, drivePullItem{RelPath: absPath, Action: "delete_failed", Error: relErr.Error()})
					failed++
					continue
				}
				rel = filepath.ToSlash(rel)
				// Consult remotePaths (every Drive entry, regardless of
				// type) rather than remoteFiles (downloadable subset
				// only). Otherwise an online doc / shortcut at e.g.
				// "notes.docx" would leave a same-named local file
				// looking orphaned and get unlinked even though Drive
				// still knows about that path.
				if _, ok := remotePaths[rel]; ok {
					continue
				}
				// FileIO has no Remove(); the absolute path comes from
				// walking safeRoot, which validate.SafeInputPath has
				// already bounded inside cwd, so a bare os.Remove is
				// acceptable here. Shortcuts cannot import internal/vfs
				// directly (depguard rule shortcuts-no-vfs).
				if err := os.Remove(absPath); err != nil { //nolint:forbidigo // see comment above
					items = append(items, drivePullItem{RelPath: rel, Action: "delete_failed", Error: err.Error()})
					failed++
					continue
				}
				items = append(items, drivePullItem{RelPath: rel, Action: "deleted_local"})
				deletedLocal++
			}
		}

		payload := map[string]interface{}{
			"summary": map[string]interface{}{
				"downloaded":    downloaded,
				"skipped":       skipped,
				"failed":        failed,
				"deleted_local": deletedLocal,
			},
			"items": items,
		}

		// Item-level failures (download error, dir/file conflict, delete
		// error) must surface as a non-zero exit so AI / script callers
		// don't have to reach into summary.failed to detect a partial
		// sync. The same structured payload rides along in error.detail
		// so forensics aren't lost. When --delete-local was skipped
		// because of an earlier download failure, callers see
		// deleted_local=0 plus the download failure that aborted it,
		// which is what makes the partial state self-explanatory.
		if failed > 0 {
			msg := fmt.Sprintf("%d item(s) failed during +pull; partial sync — re-run after resolving the failures", failed)
			if deleteLocal && downloadFailed > 0 {
				msg += " (--delete-local was skipped because the download pass had failures)"
			}
			return &output.ExitError{
				Code: output.ExitAPI,
				Detail: &output.ErrDetail{
					Type:    "partial_failure",
					Message: msg,
					Detail:  payload,
				},
			}
		}

		runtime.Out(payload, nil)
		return nil
	},
}

func drivePullDownload(ctx context.Context, runtime *common.RuntimeContext, fileToken, target string) error {
	resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
		HttpMethod: "GET",
		ApiPath:    fmt.Sprintf("/open-apis/drive/v1/files/%s/download", validate.EncodePathSegment(fileToken)),
	})
	if err != nil {
		return output.ErrNetwork("download %s: %s", common.MaskToken(fileToken), err)
	}
	defer resp.Body.Close()
	if _, err := runtime.FileIO().Save(target, fileio.SaveOptions{
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
	}, resp.Body); err != nil {
		return common.WrapSaveErrorByCategory(err, "io")
	}
	return nil
}

func drivePullRemoteViews(entries []driveRemoteEntry, duplicateRemote string) (map[string]drivePullTarget, map[string]struct{}, error) {
	remoteFiles := make(map[string]drivePullTarget, len(entries))
	remotePaths := make(map[string]struct{}, len(entries))
	fileGroups := make(map[string][]driveRemoteEntry)
	occupied := occupiedRemotePaths(entries)

	for _, entry := range entries {
		if entry.Type == driveTypeFile {
			fileGroups[entry.RelPath] = append(fileGroups[entry.RelPath], entry)
			continue
		}
		remotePaths[entry.RelPath] = struct{}{}
	}

	relPaths := make([]string, 0, len(fileGroups))
	for rel := range fileGroups {
		relPaths = append(relPaths, rel)
	}
	sort.Strings(relPaths)

	for _, rel := range relPaths {
		files := fileGroups[rel]
		if len(files) == 1 {
			remoteFiles[rel] = drivePullTarget{DownloadToken: files[0].FileToken, ItemFileToken: files[0].FileToken}
			remotePaths[rel] = struct{}{}
			continue
		}
		switch duplicateRemote {
		case driveDuplicateRemoteRename:
			candidates := append([]driveRemoteEntry(nil), files...)
			sortRemoteFiles(candidates, driveDuplicateRemoteOldest)
			for idx, file := range candidates {
				targetRel := rel
				if idx > 0 {
					var err error
					targetRel, err = relPathWithUniqueFileTokenSuffix(rel, file.FileToken, occupied)
					if err != nil {
						return nil, nil, err
					}
				}
				remoteFiles[targetRel] = drivePullTarget{
					DownloadToken: file.FileToken,
					ItemSourceID:  stableTokenIdentifier(file.FileToken),
				}
				remotePaths[targetRel] = struct{}{}
			}
		case driveDuplicateRemoteNewest, driveDuplicateRemoteOldest:
			chosen, err := chooseRemoteFile(files, duplicateRemote)
			if err != nil {
				return nil, nil, err
			}
			remoteFiles[rel] = drivePullTarget{DownloadToken: chosen.FileToken, ItemFileToken: chosen.FileToken}
			remotePaths[rel] = struct{}{}
		default:
			return nil, nil, fmt.Errorf("unsupported duplicate remote strategy %q", duplicateRemote)
		}
	}
	return remoteFiles, remotePaths, nil
}

// drivePullWalkLocal walks the canonical absolute root and returns the
// absolute paths of every regular file underneath it. The caller deletes
// some of these paths, so it is critical that they are produced by
// walking a canonical root (no symlinks in the path) — otherwise OS path
// resolution could redirect a delete to a file outside cwd. Same threat
// model as drive_status.go.
func drivePullWalkLocal(root string) ([]string, error) {
	var paths []string
	// FileIO has no walker today; shortcuts cannot import internal/vfs
	// (depguard rule shortcuts-no-vfs). The root passed in is the
	// canonical absolute path returned by validate.SafeInputPath, so
	// WalkDir's default "do not follow child symlinks" policy keeps the
	// traversal inside the validated subtree.
	err := filepath.WalkDir(root, func(absPath string, d fs.DirEntry, walkErr error) error { //nolint:forbidigo // see comment above
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		paths = append(paths, absPath)
		return nil
	})
	if err != nil {
		return nil, output.Errorf(output.ExitInternal, "io", "walk %s: %s", root, err)
	}
	return paths, nil
}
