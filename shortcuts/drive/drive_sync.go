// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	driveSyncOnConflictLocalWins  = "local-wins"
	driveSyncOnConflictRemoteWins = "remote-wins"
	driveSyncOnConflictKeepBoth   = "keep-both"
	driveSyncOnConflictAsk        = "ask"
)

type driveSyncItem struct {
	RelPath   string `json:"rel_path"`
	FileToken string `json:"file_token,omitempty"`
	Action    string `json:"action"`
	Direction string `json:"direction,omitempty"` // "pull" or "push"
	Error     string `json:"error,omitempty"`
}

// DriveSync performs a two-way sync between a local directory and a Drive
// folder. It computes a diff (like +status), then:
//   - new_remote → pull (download to local)
//   - new_local  → push (upload to Drive)
//   - modified   → resolve by --on-conflict strategy:
//     local-wins: push local over remote;
//     remote-wins: pull remote over local;
//     keep-both: rename the local file with a hash suffix and pull the remote;
//     ask: prompt the user per conflict.
var DriveSync = common.Shortcut{
	Service:     "drive",
	Command:     "+sync",
	Description: "Two-way sync between a local directory and a Drive folder",
	Risk:        "write",
	Scopes:      []string{"drive:drive.metadata:readonly"},
	ConditionalScopes: []string{
		"drive:file:download",
		"drive:file:upload",
		"space:folder:create",
	},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "local-dir", Desc: "local root directory (relative to cwd)", Required: true},
		{Name: "folder-token", Desc: "Drive folder token", Required: true},
		{Name: "on-conflict", Desc: "conflict resolution when both sides modified a file", Default: driveSyncOnConflictRemoteWins, Enum: []string{driveSyncOnConflictLocalWins, driveSyncOnConflictRemoteWins, driveSyncOnConflictKeepBoth, driveSyncOnConflictAsk}},
		{Name: "on-duplicate-remote", Desc: "policy when multiple remote Drive entries map to the same rel_path", Default: driveDuplicateRemoteFail, Enum: []string{driveDuplicateRemoteFail, driveDuplicateRemoteNewest, driveDuplicateRemoteOldest}},
		{Name: "quick", Type: "bool", Desc: "use best-effort modified_time comparison instead of SHA-256 hash; mismatched timestamps can still trigger real sync writes"},
	},
	Tips: []string{
		"Two-way sync: new remote files are pulled, new local files are pushed, and conflicts (both sides modified) are resolved by --on-conflict.",
		"Default --on-conflict=remote-wins pulls the remote version when both sides changed a file. Use local-wins to push instead, keep-both to rename and keep both copies, or ask for interactive resolution.",
		"Pass --quick for faster best-effort diff detection using modified_time instead of SHA-256 hash (no remote file downloads needed during diffing).",
		"Because +sync acts on the diff, --quick can still pull, overwrite, or rename files when timestamps differ even if file contents are actually unchanged.",
		"Only entries with type=file are synced; online docs (docx, sheet, bitable, mindnote, slides) and shortcuts are skipped.",
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
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			Desc("Compute diff between --local-dir and --folder-token, then pull new/modified-remote files, push new/modified-local files, and resolve conflicts by --on-conflict strategy.").
			GET("/open-apis/drive/v1/files").
			Set("folder_token", runtime.Str("folder-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		localDir := strings.TrimSpace(runtime.Str("local-dir"))
		folderToken := strings.TrimSpace(runtime.Str("folder-token"))
		onConflict := strings.TrimSpace(runtime.Str("on-conflict"))
		if onConflict == "" {
			onConflict = driveSyncOnConflictRemoteWins
		}
		duplicateRemote := strings.TrimSpace(runtime.Str("on-duplicate-remote"))
		if duplicateRemote == "" {
			duplicateRemote = driveDuplicateRemoteFail
		}
		quick := runtime.Bool("quick")
		if !quick {
			if err := runtime.EnsureScopes([]string{"drive:file:download"}); err != nil {
				return err
			}
		}

		safeRoot, err := validate.SafeInputPath(localDir)
		if err != nil {
			return output.ErrValidation("--local-dir: %s", err)
		}
		cwdCanonical, err := validate.SafeInputPath(".")
		if err != nil {
			return output.ErrValidation("could not resolve cwd: %s", err)
		}
		rootRelToCwd, err := filepath.Rel(cwdCanonical, safeRoot)
		if err != nil {
			return output.ErrValidation("--local-dir resolves outside cwd: %s", err)
		}

		// --- Phase 1: Compute diff (same logic as +status) ---
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
		if duplicates := blockingRemotePathConflicts(entries, duplicateRemote); len(duplicates) > 0 {
			return duplicateRemotePathError(duplicates)
		}

		// A local regular file at the same rel_path as a remote
		// folder/docx/shortcut is a type conflict: +sync would
		// classify it as new_local and attempt to upload, which either
		// fails at the API or leaves the remote in a broken state
		// (same rel_path with mixed types). Detect early and hard-fail.
		// Symmetrically, a local directory at the same rel_path as a
		// remote file/docx/shortcut would attempt create_folder and
		// produce the same broken mixed-type state.
		var typeConflicts []string
		for _, entry := range entries {
			if entry.Type == driveTypeFile {
				continue
			}
			if _, hasLocal := localFiles[entry.RelPath]; hasLocal {
				typeConflicts = append(typeConflicts, fmt.Sprintf("%q: local file vs remote %s", entry.RelPath, entry.Type))
			}
		}
		// Check local directories vs remote non-folder entries.
		// localDirs is not available yet (walked later), so check
		// the filesystem directly for the subset of remote paths
		// that are non-folder.
		for _, entry := range entries {
			if entry.Type == driveTypeFolder {
				continue
			}
			dirPath := filepath.Join(safeRoot, filepath.FromSlash(entry.RelPath))
			if info, err := os.Stat(dirPath); err == nil && info.IsDir() { //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard rule shortcuts-no-vfs); safeRoot is validated.
				typeConflicts = append(typeConflicts, fmt.Sprintf("%q: local directory vs remote %s", entry.RelPath, entry.Type))
			}
		}
		if len(typeConflicts) > 0 {
			return output.ErrValidation("+sync cannot proceed: path type conflict — %s; remove the local entry or the remote entry and retry", strings.Join(typeConflicts, "; "))
		}

		// Build the exact remote-file views that later execution will use so the
		// diff phase classifies files against the same duplicate-resolution choice.
		pullRemoteFiles, _, err := drivePullRemoteViews(entries, duplicateRemote)
		if err != nil {
			return output.Errorf(output.ExitInternal, "internal", "%s", err)
		}
		remoteEntriesForPush, remoteFolders, _, err := drivePushRemoteViews(entries, duplicateRemote)
		if err != nil {
			return output.Errorf(output.ExitInternal, "internal", "%s", err)
		}

		remoteFiles := driveSyncStatusRemoteFiles(pullRemoteFiles)

		paths := mergeStatusPaths(localFiles, remoteFiles)

		var newLocal, newRemote, modified []driveStatusEntry
		var unchanged []driveStatusEntry
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
				if quick {
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

		detection := driveStatusDetectionExact
		if quick {
			detection = driveStatusDetectionQuick
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Diff: %d new_local, %d new_remote, %d modified, %d unchanged (detection=%s)\n",
			len(newLocal), len(newRemote), len(modified), len(unchanged), detection)

		conflictResolutions := make(map[string]string, len(modified))
		if onConflict == driveSyncOnConflictAsk && len(modified) > 0 && runtime.IO().In == nil {
			return output.ErrValidation("--on-conflict=ask requires interactive stdin when modified files exist")
		}
		for _, entry := range modified {
			resolved := onConflict
			if resolved == driveSyncOnConflictAsk {
				resolved, err = driveSyncAskConflict(entry.RelPath, runtime)
				if err != nil {
					payload := map[string]interface{}{
						"detection": detection,
						"diff": map[string]interface{}{
							"new_local":  emptyIfNil(newLocal),
							"new_remote": emptyIfNil(newRemote),
							"modified":   emptyIfNil(modified),
							"unchanged":  emptyIfNil(unchanged),
						},
						"summary": map[string]interface{}{
							"pulled":  0,
							"pushed":  0,
							"skipped": 0,
							"failed":  1,
						},
						"items": []driveSyncItem{{
							RelPath:   entry.RelPath,
							FileToken: entry.FileToken,
							Action:    "failed",
							Direction: "conflict",
							Error:     err.Error(),
						}},
					}
					return &output.ExitError{
						Code: output.ExitAPI,
						Detail: &output.ErrDetail{
							Type:    "partial_failure",
							Message: fmt.Sprintf("cannot collect conflict decisions for +sync: %v", err),
							Detail:  payload,
						},
					}
				}
			}
			conflictResolutions[entry.RelPath] = resolved
		}

		// --- Phase 2: Execute sync operations ---
		var pulled, pushed, skipped, failed int
		items := make([]driveSyncItem, 0)

		if quick && driveSyncNeedsDownloadScope(newRemote, modified, conflictResolutions) {
			if err := runtime.EnsureScopes([]string{"drive:file:download"}); err != nil {
				return err
			}
		}
		plannedUploads := driveSyncPlannedUploadPaths(newLocal, modified, conflictResolutions)
		if len(plannedUploads) > 0 {
			if err := runtime.EnsureScopes([]string{"drive:file:upload"}); err != nil {
				return err
			}
		}

		// Build push infrastructure: local walk for push + remote views + folder cache.
		folderCache := map[string]string{"": folderToken}
		for relDir, entry := range remoteFolders {
			folderCache[relDir] = entry.FileToken
		}

		// Walk local filesystem early so we can include empty directories
		// in the scope preflight (they also need space:folder:create).
		pushLocalFiles, localDirs, err := drivePushWalkLocal(safeRoot, cwdCanonical)
		if err != nil {
			return err
		}

		if driveSyncNeedsCreateScope(plannedUploads, localDirs, folderCache) {
			if err := runtime.EnsureScopes([]string{"space:folder:create"}); err != nil {
				return err
			}
		}

		// Mirror local directory structure first (same as +push), so
		// empty local directories are not silently dropped.
		for _, relDir := range localDirs {
			if _, alreadyRemote := folderCache[relDir]; alreadyRemote {
				continue
			}
			if _, ensureErr := drivePushEnsureFolder(ctx, runtime, folderToken, relDir, folderCache); ensureErr != nil {
				items = append(items, driveSyncItem{RelPath: relDir, Action: "failed", Direction: "push", Error: ensureErr.Error()})
				failed++
				continue
			}
			items = append(items, driveSyncItem{RelPath: relDir, FileToken: folderCache[relDir], Action: "folder_created", Direction: "push"})
			pushed++
		}

		// 2a. Pull new_remote files.
		for _, entry := range newRemote {
			targetFile, ok := pullRemoteFiles[entry.RelPath]
			if !ok {
				// Non-file type (doc, shortcut, etc.) — skip.
				continue
			}
			target := filepath.Join(rootRelToCwd, entry.RelPath)
			if err := drivePullDownload(ctx, runtime, targetFile.DownloadToken, target, targetFile.ModifiedTime); err != nil {
				items = append(items, driveSyncItem{RelPath: entry.RelPath, FileToken: entry.FileToken, Action: "failed", Direction: "pull", Error: err.Error()})
				failed++
				continue
			}
			items = append(items, driveSyncItem{RelPath: entry.RelPath, FileToken: entry.FileToken, Action: "downloaded", Direction: "pull"})
			pulled++
		}

		// 2b. Push new_local files.
		for _, entry := range newLocal {
			localFile, ok := pushLocalFiles[entry.RelPath]
			if !ok {
				items = append(items, driveSyncItem{RelPath: entry.RelPath, Action: "skipped", Direction: "push", Error: "local file disappeared during sync"})
				skipped++
				continue
			}
			parentRel := drivePushParentRel(entry.RelPath)
			parentToken, ensureErr := drivePushEnsureFolder(ctx, runtime, folderToken, parentRel, folderCache)
			if ensureErr != nil {
				items = append(items, driveSyncItem{RelPath: entry.RelPath, Action: "failed", Direction: "push", Error: ensureErr.Error()})
				failed++
				continue
			}
			token, _, upErr := drivePushUploadFile(ctx, runtime, localFile, "", parentToken)
			if upErr != nil {
				items = append(items, driveSyncItem{RelPath: entry.RelPath, Action: "failed", Direction: "push", Error: upErr.Error()})
				failed++
				continue
			}
			items = append(items, driveSyncItem{RelPath: entry.RelPath, FileToken: token, Action: "uploaded", Direction: "push"})
			pushed++
		}

		// 2c. Resolve modified files by --on-conflict strategy.
		for _, entry := range modified {
			remoteFile := remoteFiles[entry.RelPath]
			localFile, hasLocal := pushLocalFiles[entry.RelPath]
			if !hasLocal {
				// Should not happen — modified means both sides exist.
				items = append(items, driveSyncItem{RelPath: entry.RelPath, Action: "skipped", Direction: "conflict", Error: "local file disappeared during sync"})
				skipped++
				continue
			}

			resolved := conflictResolutions[entry.RelPath]
			if resolved == "" {
				items = append(items, driveSyncItem{RelPath: entry.RelPath, Action: "skipped", Direction: "conflict", Error: "user skipped"})
				skipped++
				continue
			}

			switch resolved {
			case driveSyncOnConflictRemoteWins:
				// Pull remote over local.
				targetFile, ok := pullRemoteFiles[entry.RelPath]
				if !ok {
					items = append(items, driveSyncItem{RelPath: entry.RelPath, Action: "failed", Direction: "pull", Error: "remote file not found in pull views"})
					failed++
					continue
				}
				target := filepath.Join(rootRelToCwd, entry.RelPath)
				if err := drivePullDownload(ctx, runtime, targetFile.DownloadToken, target, targetFile.ModifiedTime); err != nil {
					items = append(items, driveSyncItem{RelPath: entry.RelPath, FileToken: entry.FileToken, Action: "failed", Direction: "pull", Error: err.Error()})
					failed++
					continue
				}
				items = append(items, driveSyncItem{RelPath: entry.RelPath, FileToken: entry.FileToken, Action: "downloaded", Direction: "pull"})
				pulled++

			case driveSyncOnConflictLocalWins:
				// Push local over remote.
				existingToken := remoteFile.FileToken
				if existingToken == "" {
					if chosen, ok := remoteEntriesForPush[entry.RelPath]; ok {
						existingToken = chosen.FileToken
					}
				}
				parentToken, parentErr := drivePushEnsureFolder(ctx, runtime, folderToken, drivePushParentRel(entry.RelPath), folderCache)
				if parentErr != nil {
					items = append(items, driveSyncItem{RelPath: entry.RelPath, FileToken: existingToken, Action: "failed", Direction: "push", Error: parentErr.Error()})
					failed++
					continue
				}
				token, _, upErr := drivePushUploadFile(ctx, runtime, localFile, existingToken, parentToken)
				if upErr != nil {
					// Token contract on overwrite failure (same as +push):
					// a partial-success response can return a non-empty
					// file_token alongside an error. Prefer the freshly
					// returned token when one was produced, fall back to
					// existingToken otherwise.
					failedToken := token
					if failedToken == "" {
						failedToken = existingToken
					}
					items = append(items, driveSyncItem{RelPath: entry.RelPath, FileToken: failedToken, Action: "failed", Direction: "push", Error: upErr.Error()})
					failed++
					continue
				}
				items = append(items, driveSyncItem{RelPath: entry.RelPath, FileToken: token, Action: "overwritten", Direction: "push"})
				pushed++

			case driveSyncOnConflictKeepBoth:
				// Rename the local file with a hash suffix, then pull the remote.
				// Use the remote file token to generate a stable suffix (same
				// pattern as +pull --on-duplicate-remote=rename).
				occupied := occupiedRemotePaths(entries)
				// Add current local paths to occupied set so the renamed
				// local file doesn't collide with an existing file or directory.
				for p := range pushLocalFiles {
					occupied[p] = struct{}{}
				}
				for _, relDir := range localDirs {
					occupied[relDir] = struct{}{}
				}
				suffixedRel, err := relPathWithUniqueFileTokenSuffix(entry.RelPath, remoteFile.FileToken, occupied)
				if err != nil {
					items = append(items, driveSyncItem{RelPath: entry.RelPath, Action: "failed", Direction: "conflict", Error: err.Error()})
					failed++
					continue
				}
				// Rename the local file.
				oldAbsPath := filepath.Join(safeRoot, filepath.FromSlash(entry.RelPath))
				newAbsPath := filepath.Join(safeRoot, filepath.FromSlash(suffixedRel))
				if err := os.Rename(oldAbsPath, newAbsPath); err != nil { //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard rule shortcuts-no-vfs); safeRoot is validated.
					items = append(items, driveSyncItem{RelPath: entry.RelPath, Action: "failed", Direction: "conflict", Error: fmt.Sprintf("rename local: %s", err)})
					failed++
					continue
				}
				occupied[suffixedRel] = struct{}{}
				// Now pull the remote version to the original path.
				targetFile, ok := pullRemoteFiles[entry.RelPath]
				if !ok {
					rollbackErr := driveSyncRollbackRenamedLocal(oldAbsPath, newAbsPath)
					errMsg := "remote file not found in pull views after rename"
					if rollbackErr != nil {
						errMsg += "; rollback failed: " + rollbackErr.Error()
					}
					items = append(items, driveSyncItem{RelPath: entry.RelPath, Action: "failed", Direction: "pull", Error: errMsg})
					failed++
					continue
				}
				target := filepath.Join(rootRelToCwd, entry.RelPath)
				if err := drivePullDownload(ctx, runtime, targetFile.DownloadToken, target, targetFile.ModifiedTime); err != nil {
					rollbackErr := driveSyncRollbackRenamedLocal(oldAbsPath, newAbsPath)
					errMsg := err.Error()
					if rollbackErr != nil {
						errMsg += "; rollback failed: " + rollbackErr.Error()
					}
					items = append(items, driveSyncItem{RelPath: entry.RelPath, FileToken: entry.FileToken, Action: "failed", Direction: "pull", Error: errMsg})
					failed++
					continue
				}
				items = append(items, driveSyncItem{RelPath: entry.RelPath, Action: "renamed_local", Direction: "conflict"})
				items = append(items, driveSyncItem{RelPath: entry.RelPath, FileToken: entry.FileToken, Action: "downloaded", Direction: "pull"})
				pulled++

			default:
				items = append(items, driveSyncItem{RelPath: entry.RelPath, Action: "skipped", Direction: "conflict", Error: fmt.Sprintf("unknown conflict strategy: %s", resolved)})
				skipped++
			}
		}

		payload := map[string]interface{}{
			"detection": detection,
			"diff": map[string]interface{}{
				"new_local":  emptyIfNil(newLocal),
				"new_remote": emptyIfNil(newRemote),
				"modified":   emptyIfNil(modified),
				"unchanged":  emptyIfNil(unchanged),
			},
			"summary": map[string]interface{}{
				"pulled":  pulled,
				"pushed":  pushed,
				"skipped": skipped,
				"failed":  failed,
			},
			"items": items,
		}

		if failed > 0 {
			msg := fmt.Sprintf("%d item(s) failed during +sync", failed)
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

func driveSyncStatusRemoteFiles(pullRemoteFiles map[string]drivePullTarget) map[string]driveStatusRemoteFile {
	remoteFiles := make(map[string]driveStatusRemoteFile, len(pullRemoteFiles))
	for relPath, target := range pullRemoteFiles {
		fileToken := target.ItemFileToken
		if fileToken == "" {
			fileToken = target.DownloadToken
		}
		remoteFiles[relPath] = driveStatusRemoteFile{FileToken: fileToken, ModifiedTime: target.ModifiedTime}
	}
	return remoteFiles
}

// driveSyncAskConflict prompts the user for a conflict resolution strategy
// for a single file. Returns the strategy string, or empty string if the
// user chose to skip.
func driveSyncAskConflict(relPath string, runtime *common.RuntimeContext) (string, error) {
	fmt.Fprintf(runtime.IO().ErrOut, "CONFLICT: both sides modified %q. Choose: [R]emote-wins / [L]ocal-wins / [K]eep-both / [S]kip (default: R): ", relPath)
	if runtime.IO().In == nil {
		return "", output.ErrValidation("cannot resolve conflict for %q with --on-conflict=ask: stdin is not available", relPath)
	}
	reader, ok := runtime.IO().In.(*bufio.Reader)
	if !ok {
		reader = bufio.NewReader(runtime.IO().In)
		runtime.IO().In = reader
	}
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", output.ErrValidation("cannot read conflict choice for %q: %s", relPath, err)
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	if answer == "" {
		if errors.Is(err, io.EOF) {
			return "", output.ErrValidation("cannot resolve conflict for %q with --on-conflict=ask: stdin reached EOF before any choice was provided", relPath)
		}
		return driveSyncOnConflictRemoteWins, nil
	}
	switch answer {
	case "l", "local", "local-wins":
		return driveSyncOnConflictLocalWins, nil
	case "k", "keep", "keep-both":
		return driveSyncOnConflictKeepBoth, nil
	case "s", "skip":
		return "", nil
	case "r", "remote", "remote-wins":
		return driveSyncOnConflictRemoteWins, nil
	default:
		return "", output.ErrValidation("invalid conflict choice for %q: %q (expected one of remote/local/keep/skip)", relPath, strings.TrimSpace(line))
	}
}

func driveSyncNeedsDownloadScope(newRemote, modified []driveStatusEntry, conflictResolutions map[string]string) bool {
	if len(newRemote) > 0 {
		return true
	}
	for _, entry := range modified {
		switch conflictResolutions[entry.RelPath] {
		case driveSyncOnConflictRemoteWins, driveSyncOnConflictKeepBoth:
			return true
		}
	}
	return false
}

func driveSyncPlannedUploadPaths(newLocal, modified []driveStatusEntry, conflictResolutions map[string]string) []string {
	planned := make([]string, 0, len(newLocal)+len(modified))
	for _, entry := range newLocal {
		planned = append(planned, entry.RelPath)
	}
	for _, entry := range modified {
		if conflictResolutions[entry.RelPath] == driveSyncOnConflictLocalWins {
			planned = append(planned, entry.RelPath)
		}
	}
	return planned
}

func driveSyncNeedsCreateScope(uploadPaths []string, localDirs []string, folderCache map[string]string) bool {
	for _, relPath := range uploadPaths {
		parentRel := drivePushParentRel(relPath)
		if parentRel == "" {
			continue
		}
		if _, ok := folderCache[parentRel]; !ok {
			return true
		}
	}
	// Empty local directories also need create_folder if not already on Drive.
	for _, relDir := range localDirs {
		if _, ok := folderCache[relDir]; !ok {
			return true
		}
	}
	return false
}

func driveSyncRollbackRenamedLocal(oldAbsPath, newAbsPath string) error {
	if info, err := os.Stat(oldAbsPath); err == nil { //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard rule shortcuts-no-vfs); safeRoot is validated.
		if info.IsDir() {
			return output.Errorf(output.ExitInternal, "rollback", "original path became a directory during rollback: %s", oldAbsPath)
		}
		if err := os.Remove(oldAbsPath); err != nil { //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard rule shortcuts-no-vfs); safeRoot is validated.
			return output.Errorf(output.ExitInternal, "rollback", "remove partial restored path %q: %s", oldAbsPath, err)
		}
	} else if !os.IsNotExist(err) {
		return output.Errorf(output.ExitInternal, "rollback", "stat original path %q during rollback: %s", oldAbsPath, err)
	}
	if err := os.Rename(newAbsPath, oldAbsPath); err != nil { //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard rule shortcuts-no-vfs); safeRoot is validated.
		return output.Errorf(output.ExitInternal, "rollback", "restore renamed local file %q: %s", oldAbsPath, err)
	}
	return nil
}
