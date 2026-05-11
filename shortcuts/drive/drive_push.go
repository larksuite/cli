// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	drivePushIfExistsOverwrite = "overwrite"
	drivePushIfExistsSkip      = "skip"
)

type drivePushItem struct {
	RelPath   string `json:"rel_path"`
	FileToken string `json:"file_token,omitempty"`
	Action    string `json:"action"`
	Version   string `json:"version,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Error     string `json:"error,omitempty"`
}

// DrivePush is a one-way, file-level mirror from a local directory onto a
// Drive folder: walks --local-dir, recursively lists --folder-token, and for
// each rel_path uploads (or overwrites) the corresponding Drive file. With
// --delete-remote --yes, any type=file entry on Drive that has no local
// counterpart is removed; online docs (docx/sheet/bitable/...), shortcuts
// and folders are never deleted, so this is "file-level" mirror — the
// command does not attempt to remove remote-only directories or close gaps
// in directory structure that exists on Drive but not locally.
//
// Only Drive entries with type=file participate in upload/overwrite/delete;
// online documents have no equivalent local binary. Sub-folders are created
// on Drive on demand via /open-apis/drive/v1/files/create_folder so the
// remote tree mirrors the local tree.
//
// The overwrite path passes the existing file_token as a form field on
// /open-apis/drive/v1/files/upload_all, mirroring the markdown +overwrite
// contract in shortcuts/markdown. The Drive backend exposing that field is
// being rolled out; until rollout completes, --if-exists defaults to "skip"
// so the safe path (do not touch existing remote files) is the default and
// callers must opt into "overwrite" explicitly.
var DrivePush = common.Shortcut{
	Service:     "drive",
	Command:     "+push",
	Description: "File-level mirror of a local directory onto a Drive folder (local → Drive; remote-only directories are not removed)",
	Risk:        "write",
	// Narrowed scopes follow the precedent set by drive +status / +pull:
	// drive:drive is policy-disabled in some tenants, so this shortcut sticks
	// to the smallest set the *core* path needs. space:folder:create is
	// always declared because mirroring a non-flat tree calls
	// /open-apis/drive/v1/files/create_folder on demand and we want the
	// framework's pre-flight scope check to catch missing grants before any
	// upload — otherwise a partial push could land top-level files and then
	// trip on a missing folder grant for a sub-tree, leaving a half-synced
	// state.
	//
	// space:document:delete is intentionally NOT in the default set even
	// though --delete-remote needs it. The framework pre-check (runner.go
	// checkShortcutScopes) runs unconditionally before Validate / dry-run,
	// so declaring it here would make every plain push (and every
	// --dry-run) fail for callers that only granted upload scopes.
	//
	// Instead, Validate runs a *conditional* pre-flight via
	// runtime.EnsureScopes when both --delete-remote and --yes are on, so
	// the missing grant fails the run upfront — before any upload —
	// rather than landing files first and tripping on missing_scope when
	// the cleanup pass tries to delete. That avoids the half-synced state
	// (files uploaded, orphans never cleaned up) that the unconditional
	// pre-check would otherwise prevent only by also blocking plain
	// pushes.
	Scopes:    []string{"drive:drive.metadata:readonly", "drive:file:upload", "space:folder:create"},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "local-dir", Desc: "local root directory (relative to cwd)", Required: true},
		{Name: "folder-token", Desc: "target Drive folder token", Required: true},
		{Name: "if-exists", Desc: "policy when a Drive file already exists at the same rel_path (default: skip — safe; opt into overwrite explicitly while the backend version field is rolling out)", Default: drivePushIfExistsSkip, Enum: []string{drivePushIfExistsOverwrite, drivePushIfExistsSkip}},
		{Name: "on-duplicate-remote", Desc: "policy when multiple remote Drive entries map to the same rel_path", Default: driveDuplicateRemoteFail, Enum: []string{driveDuplicateRemoteFail, driveDuplicateRemoteNewest, driveDuplicateRemoteOldest}},
		{Name: "delete-remote", Type: "bool", Desc: "delete Drive files absent locally (file-level mirror; remote-only directories are not removed); requires --yes"},
		{Name: "yes", Type: "bool", Desc: "confirm --delete-remote before deleting Drive files"},
	},
	Tips: []string{
		"This is a file-level mirror: only type=file entries are uploaded, overwritten or deleted. Online docs (docx, sheet, bitable, mindnote, slides), shortcuts, and remote-only directories are never touched.",
		"Local directory structure (including empty directories) is mirrored to Drive via create_folder; existing remote folders are reused.",
		"Duplicate remote rel_path conflicts fail by default before upload, overwrite, or delete. Use --on-duplicate-remote=newest|oldest only when the conflict is duplicate files and you explicitly want to target one.",
		"Default --if-exists=skip is the safe choice while the upload_all overwrite-version field is rolling out. Pass --if-exists=overwrite to replace remote bytes; on tenants without the field it surfaces a structured api_error and the run exits non-zero.",
		"--delete-remote requires --yes; without --yes the command is rejected upfront so a stray flag never deletes anything.",
		"--delete-remote --yes also requires the space:document:delete scope. Validate runs a dynamic pre-flight check when the flag is on, so a missing grant fails the run before any upload — preventing a half-synced state where files were uploaded but the cleanup pass cannot delete.",
		"Item-level failures (upload, overwrite, folder, delete) bump summary.failed and the run exits non-zero. If any upload or folder step fails, the --delete-remote phase is skipped entirely so a partial upload never triggers remote deletion.",
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
		if runtime.Bool("delete-remote") && !runtime.Bool("yes") {
			return output.ErrValidation("--delete-remote requires --yes (high-risk: deletes Drive files absent locally)")
		}
		// Conditional scope pre-check: when --delete-remote --yes is set, the
		// run will issue DELETE /open-apis/drive/v1/files/<token> after the
		// upload phase. The default Scopes list intentionally omits
		// space:document:delete so plain pushes don't get blocked on a grant
		// they don't need (see the Scopes block above), but at this point we
		// know the run will need it — pre-flight here so a missing grant
		// fails before any upload, instead of after, which would otherwise
		// leave the tenant in a half-synced state (files uploaded, remote
		// orphans never cleaned up). EnsureScopes is a silent no-op when no
		// token / scope metadata is available, so test envs and tenants
		// where the resolver doesn't expose scopes still proceed and rely on
		// the API-level missing_scope error.
		if runtime.Bool("delete-remote") && runtime.Bool("yes") {
			if err := runtime.EnsureScopes([]string{"space:document:delete"}); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			Desc("Walk --local-dir, recursively list --folder-token, then upload new files, overwrite (when --if-exists=overwrite) or skip existing, and (when --delete-remote --yes is set) delete Drive files absent locally.").
			GET("/open-apis/drive/v1/files").
			Set("folder_token", runtime.Str("folder-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		localDir := strings.TrimSpace(runtime.Str("local-dir"))
		folderToken := strings.TrimSpace(runtime.Str("folder-token"))
		ifExists := strings.TrimSpace(runtime.Str("if-exists"))
		if ifExists == "" {
			// Default to the safe "skip" policy: do not touch already-present
			// remote files. Callers must pass --if-exists=overwrite to opt
			// into the overwrite-with-version path that depends on the
			// rolling-out upload_all `file_token`/`version` protocol field.
			ifExists = drivePushIfExistsSkip
		}
		duplicateRemote := strings.TrimSpace(runtime.Str("on-duplicate-remote"))
		if duplicateRemote == "" {
			duplicateRemote = driveDuplicateRemoteFail
		}
		deleteRemote := runtime.Bool("delete-remote")

		// Resolve --local-dir to its canonical absolute path before walking.
		// SafeInputPath fully evaluates symlinks across the entire path,
		// which closes the kernel-level escape route that filepath.Clean
		// alone misses (e.g. "link/.." string-cleans to "." but the kernel
		// resolves through link's target's parent). Walking the canonical
		// root sidesteps that, and the matching cwd canonical lets each
		// absolute walk hit be converted to a cwd-relative path that
		// FileIO.Open's SafeInputPath check still accepts.
		safeRoot, err := validate.SafeInputPath(localDir)
		if err != nil {
			return output.ErrValidation("--local-dir: %s", err)
		}
		cwdCanonical, err := validate.SafeInputPath(".")
		if err != nil {
			return output.ErrValidation("could not resolve cwd: %s", err)
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Walking local: %s\n", localDir)
		localFiles, localDirs, err := drivePushWalkLocal(safeRoot, cwdCanonical)
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
		// Two views over the same listing:
		//   - remoteFiles drives upload / overwrite / orphan-delete
		//     decisions (only type=file entries are upload candidates;
		//     online docs / shortcuts are intentionally never overwritten
		//     or deleted by --delete-remote).
		//   - remoteFolders is the create_folder cache: lets the upload
		//     path skip create_folder when an intermediate folder already
		//     exists, and keeps directory recreation idempotent across
		//     reruns.
		remoteFiles, remoteFolders, remoteFileGroups, err := drivePushRemoteViews(entries, duplicateRemote)
		if err != nil {
			return output.Errorf(output.ExitInternal, "internal", "%s", err)
		}

		var uploaded, skipped, failed, deletedRemote int
		items := make([]drivePushItem, 0)
		// uploadFailed tracks whether any folder-creation, upload or
		// overwrite step failed. The --delete-remote phase only runs when
		// this stays false: a partial upload that then proceeds to delete
		// remote orphans would leave the tenant half-synced (files missing
		// locally and now on Drive too), which is the worst-of-both-worlds
		// outcome the review flagged.
		uploadFailed := false

		// folderCache holds rel_path → folder_token. Seeded from the remote
		// listing (so we don't recreate folders that already exist) and
		// extended in-place as drivePushEnsureFolder mints new ones.
		folderCache := map[string]string{"": folderToken}
		for relDir, entry := range remoteFolders {
			folderCache[relDir] = entry.FileToken
		}

		// Mirror local directory structure first, so empty directories
		// are not silently dropped. Pre-creating also frees the upload
		// loop from doing on-demand mkdir for every file's parent chain
		// (the cache makes both paths idempotent, but pre-creation keeps
		// items[] in a tidy "folders, then files" shape).
		for _, relDir := range localDirs {
			if _, alreadyRemote := folderCache[relDir]; alreadyRemote {
				// Folder already exists on Drive — nothing to do; staying
				// silent (no items[] entry) avoids noise on reruns.
				continue
			}
			if _, ensureErr := drivePushEnsureFolder(ctx, runtime, folderToken, relDir, folderCache); ensureErr != nil {
				items = append(items, drivePushItem{RelPath: relDir, Action: "failed", Error: ensureErr.Error()})
				failed++
				uploadFailed = true
				continue
			}
			items = append(items, drivePushItem{RelPath: relDir, FileToken: folderCache[relDir], Action: "folder_created"})
		}

		// Upload local-only and overwrite/skip already-present files in a
		// stable order so output is reproducible.
		localPaths := make([]string, 0, len(localFiles))
		for p := range localFiles {
			localPaths = append(localPaths, p)
		}
		sort.Strings(localPaths)

		for _, rel := range localPaths {
			localFile := localFiles[rel]

			if entry, ok := remoteFiles[rel]; ok {
				if ifExists == drivePushIfExistsSkip {
					items = append(items, drivePushItem{RelPath: rel, FileToken: entry.FileToken, Action: "skipped", SizeBytes: localFile.Size})
					skipped++
					continue
				}
				token, version, upErr := drivePushUploadFile(ctx, runtime, localFile, entry.FileToken, folderToken)
				if upErr != nil {
					// Token contract on overwrite failure: an in-place
					// overwrite preserves the file's token, so the
					// existing entry.FileToken is normally still the
					// authoritative pointer to the (possibly already
					// rewritten) Drive file. But the protocol does not
					// strictly forbid the backend from minting a new
					// token, and a partial-success response can return a
					// non-empty file_token alongside an error (the
					// missing-version case below is the immediate
					// concern: bytes hit the disk, version field
					// missing, so we surface a structured error). Prefer
					// the freshly returned token when one was produced,
					// fall back to entry.FileToken otherwise — that way
					// callers still have a usable handle to whatever
					// state Drive ended up in.
					failedToken := token
					if failedToken == "" {
						failedToken = entry.FileToken
					}
					items = append(items, drivePushItem{RelPath: rel, FileToken: failedToken, Action: "failed", SizeBytes: localFile.Size, Error: upErr.Error()})
					failed++
					uploadFailed = true
					continue
				}
				items = append(items, drivePushItem{RelPath: rel, FileToken: token, Action: "overwritten", Version: version, SizeBytes: localFile.Size})
				uploaded++
				continue
			}

			parentRel := drivePushParentRel(rel)
			parentToken, ensureErr := drivePushEnsureFolder(ctx, runtime, folderToken, parentRel, folderCache)
			if ensureErr != nil {
				items = append(items, drivePushItem{RelPath: rel, Action: "failed", SizeBytes: localFile.Size, Error: ensureErr.Error()})
				failed++
				uploadFailed = true
				continue
			}
			token, _, upErr := drivePushUploadFile(ctx, runtime, localFile, "", parentToken)
			if upErr != nil {
				items = append(items, drivePushItem{RelPath: rel, Action: "failed", SizeBytes: localFile.Size, Error: upErr.Error()})
				failed++
				uploadFailed = true
				continue
			}
			items = append(items, drivePushItem{RelPath: rel, FileToken: token, Action: "uploaded", SizeBytes: localFile.Size})
			uploaded++
		}

		// Skip the delete phase entirely on any upstream failure. The orphan
		// loop deletes by remote token and is unrecoverable; running it
		// after a failed upload risks deleting a file the partial upload
		// would have replaced on a successful re-run, leaving the tenant
		// in a worse state than where we started. Surface the skipped
		// delete as a hint in stderr so operators know the cleanup pass
		// is pending and can re-run after fixing the upload.
		if deleteRemote && uploadFailed {
			fmt.Fprintf(runtime.IO().ErrOut,
				"Skipping --delete-remote: %d earlier failure(s) — re-run after resolving them.\n",
				failed)
		}
		if deleteRemote && !uploadFailed {
			// Stable iteration order so failures (and tests) are deterministic.
			remoteRelPaths := make([]string, 0, len(remoteFileGroups))
			for p := range remoteFileGroups {
				remoteRelPaths = append(remoteRelPaths, p)
			}
			sort.Strings(remoteRelPaths)

			for _, rel := range remoteRelPaths {
				keepToken := ""
				if _, ok := localFiles[rel]; ok {
					if chosen, ok := remoteFiles[rel]; ok {
						keepToken = chosen.FileToken
					}
				}
				for _, entry := range remoteFileGroups[rel] {
					if entry.FileToken == keepToken {
						continue
					}
					if err := drivePushDeleteFile(ctx, runtime, entry.FileToken); err != nil {
						items = append(items, drivePushItem{RelPath: rel, FileToken: entry.FileToken, Action: "delete_failed", Error: err.Error()})
						failed++
						continue
					}
					items = append(items, drivePushItem{RelPath: rel, FileToken: entry.FileToken, Action: "deleted_remote"})
					deletedRemote++
				}
			}
		}

		runtime.Out(map[string]interface{}{
			"summary": map[string]interface{}{
				"uploaded":       uploaded,
				"skipped":        skipped,
				"failed":         failed,
				"deleted_remote": deletedRemote,
			},
			"items": items,
		}, nil)
		// Bump the exit code on any item-level failure (upload, overwrite,
		// folder, or delete) so callers / scripts / agents can react. The
		// summary + items[] envelope was just written to stdout via Out(),
		// so ErrBare here only affects the exit code — the structured
		// per-item context is still in the stdout JSON.
		if failed > 0 {
			return output.ErrBare(output.ExitAPI)
		}
		return nil
	},
}

// drivePushLocalFile records what we need to upload a local regular file:
// a rel_path used for output and Drive layout, the cwd-relative path that
// FileIO.Open accepts, the file size (drives single/multipart selection),
// and the basename used as Drive's file_name.
type drivePushLocalFile struct {
	RelPath  string
	OpenPath string
	FileName string
	Size     int64
}

// drivePushWalkLocal walks the canonical absolute root produced by
// SafeInputPath. Same threat model as +pull/+status: the validated root
// is not a symlink itself, and WalkDir's default policy (do not follow
// child symlinks) keeps the traversal inside that canonical subtree, so
// the OpenPath we hand to FileIO.Open stays inside cwd.
//
// Returns two views:
//   - files: rel_path → file metadata; drives the upload/skip/overwrite loop.
//   - dirs:  every non-root directory rel_path encountered. Used to mirror
//     empty directories (which would otherwise be silently dropped because
//     the upload loop only iterates files); non-empty directories appear
//     here too but are harmless because drivePushEnsureFolder is cached.
func drivePushWalkLocal(root, cwdCanonical string) (map[string]drivePushLocalFile, []string, error) {
	files := make(map[string]drivePushLocalFile)
	dirsSet := make(map[string]struct{})
	// FileIO has no walker today and shortcuts can't import internal/vfs
	// (depguard rule shortcuts-no-vfs). The walk root is the canonical
	// absolute path returned by validate.SafeInputPath, so it is no
	// longer a symlink itself, and WalkDir's default child-symlink
	// policy keeps the traversal inside the validated subtree.
	err := filepath.WalkDir(root, func(absPath string, d fs.DirEntry, walkErr error) error { //nolint:forbidigo // see comment above
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, absPath)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if d.IsDir() {
			// Skip the root itself ("."): that is --folder-token, already
			// the parent we mirror into, not a sub-folder we need to
			// create.
			if relSlash != "." {
				dirsSet[relSlash] = struct{}{}
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		relToCwd, err := filepath.Rel(cwdCanonical, absPath)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files[relSlash] = drivePushLocalFile{
			RelPath:  relSlash,
			OpenPath: relToCwd,
			FileName: filepath.Base(rel),
			Size:     info.Size(),
		}
		return nil
	})
	if err != nil {
		return nil, nil, output.Errorf(output.ExitInternal, "io", "walk %s: %s", root, err)
	}
	dirs := make([]string, 0, len(dirsSet))
	for d := range dirsSet {
		dirs = append(dirs, d)
	}
	// Shallow-first ordering ensures parents are created before children;
	// drivePushEnsureFolder also handles parent recursion on its own, but
	// emitting items[] in shallow-first order matches what users expect.
	sort.Slice(dirs, func(i, j int) bool {
		di, dj := strings.Count(dirs[i], "/"), strings.Count(dirs[j], "/")
		if di != dj {
			return di < dj
		}
		return dirs[i] < dirs[j]
	})
	return files, dirs, nil
}

func drivePushRemoteViews(entries []driveRemoteEntry, duplicateRemote string) (map[string]driveRemoteEntry, map[string]driveRemoteEntry, map[string][]driveRemoteEntry, error) {
	remoteFiles := make(map[string]driveRemoteEntry, len(entries))
	remoteFolders := make(map[string]driveRemoteEntry, len(entries))
	fileGroups := make(map[string][]driveRemoteEntry)

	for _, entry := range entries {
		switch entry.Type {
		case driveTypeFile:
			fileGroups[entry.RelPath] = append(fileGroups[entry.RelPath], entry)
		case driveTypeFolder:
			remoteFolders[entry.RelPath] = entry
		}
	}

	relPaths := make([]string, 0, len(fileGroups))
	for rel := range fileGroups {
		relPaths = append(relPaths, rel)
	}
	sort.Strings(relPaths)

	for _, rel := range relPaths {
		files := fileGroups[rel]
		if len(files) == 1 {
			remoteFiles[rel] = files[0]
			continue
		}
		switch duplicateRemote {
		case driveDuplicateRemoteNewest, driveDuplicateRemoteOldest:
			chosen, err := chooseRemoteFile(files, duplicateRemote)
			if err != nil {
				return nil, nil, nil, err
			}
			remoteFiles[rel] = chosen
		default:
			return nil, nil, nil, fmt.Errorf("unsupported duplicate remote strategy %q", duplicateRemote)
		}
	}
	return remoteFiles, remoteFolders, fileGroups, nil
}

// drivePushEnsureFolder ensures a folder chain (rel_dir relative to the root
// folder identified by rootFolderToken) exists on Drive, creating any
// missing segments via /open-apis/drive/v1/files/create_folder. Returns the
// token of the deepest folder, suitable as parent_node for the upload.
//
// folderCache is shared with the caller so each segment is only created
// once per push, and so subsequent uploads under the same sub-tree reuse
// the freshly minted folder token without an extra round trip.
func drivePushEnsureFolder(ctx context.Context, runtime *common.RuntimeContext, rootFolderToken, relDir string, folderCache map[string]string) (string, error) {
	if token, ok := folderCache[relDir]; ok {
		return token, nil
	}
	parentRel, name := drivePushSplitRel(relDir)
	parentToken, err := drivePushEnsureFolder(ctx, runtime, rootFolderToken, parentRel, folderCache)
	if err != nil {
		return "", err
	}

	data, err := runtime.CallAPI(
		"POST",
		"/open-apis/drive/v1/files/create_folder",
		nil,
		map[string]interface{}{
			"name":         name,
			"folder_token": parentToken,
		},
	)
	if err != nil {
		return "", err
	}
	token := common.GetString(data, "token")
	if token == "" {
		return "", output.Errorf(output.ExitAPI, "api_error", "create_folder for %q returned no folder token", relDir)
	}
	folderCache[relDir] = token
	return token, nil
}

// drivePushUploadFile uploads (or overwrites) a single local file. When
// existingToken is non-empty, the request adds the file_token form field to
// trigger overwrite-with-version semantics on the backend; the response is
// expected to carry a non-empty `version`, which is propagated to the
// caller for the items[].version field. When existingToken is empty, this
// is a fresh upload under parentToken.
//
// Files larger than common.MaxDriveMediaUploadSinglePartSize fall back to
// the three-step prepare/part/finish flow, which mirrors drive +upload's
// existing multipart logic.
func drivePushUploadFile(ctx context.Context, runtime *common.RuntimeContext, file drivePushLocalFile, existingToken, parentToken string) (string, string, error) {
	if file.Size > common.MaxDriveMediaUploadSinglePartSize {
		token, err := drivePushUploadMultipart(ctx, runtime, file, existingToken, parentToken)
		// Multipart finish does not return version on the existing
		// /open-apis/drive/v1/files/upload_finish contract; surface an
		// empty version in that case rather than fabricating one. The
		// markdown +overwrite path has the same gap and is tracked for a
		// follow-up once the multipart endpoint exposes the field.
		return token, "", err
	}
	return drivePushUploadAll(ctx, runtime, file, existingToken, parentToken)
}

func drivePushUploadAll(_ context.Context, runtime *common.RuntimeContext, file drivePushLocalFile, existingToken, parentToken string) (string, string, error) {
	f, err := runtime.FileIO().Open(file.OpenPath)
	if err != nil {
		return "", "", common.WrapInputStatError(err)
	}
	defer f.Close()

	fd := larkcore.NewFormdata()
	fd.AddField("file_name", file.FileName)
	fd.AddField("parent_type", driveUploadParentTypeExplorer)
	fd.AddField("parent_node", parentToken)
	fd.AddField("size", fmt.Sprintf("%d", file.Size))
	if existingToken != "" {
		// Overwrite mode: the backend interprets a non-empty file_token on
		// upload_all as "replace this file's content and bump its version",
		// matching the markdown +overwrite contract.
		fd.AddField("file_token", existingToken)
	}
	fd.AddFile("file", f)

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/drive/v1/files/upload_all",
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		var exitErr *output.ExitError
		if errors.As(err, &exitErr) {
			return "", "", err
		}
		return "", "", output.ErrNetwork("upload failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return "", "", output.Errorf(output.ExitAPI, "api_error", "upload failed: invalid response JSON: %v", err)
	}
	// Extract the token before the larkCode check: the backend can produce
	// a partial-success response (code != 0 alongside a non-empty
	// data.file_token) where bytes have already landed under that token.
	// Returning "" here would force the caller to fall back to
	// entry.FileToken and silently lose the token Drive actually used,
	// defeating the overwrite-error token-stability handling in Execute.
	data, _ := result["data"].(map[string]interface{})
	token := common.GetString(data, "file_token")
	if larkCode := int(common.GetFloat(result, "code")); larkCode != 0 {
		msg, _ := result["msg"].(string)
		return token, "", output.ErrAPI(larkCode, fmt.Sprintf("upload failed: [%d] %s", larkCode, msg), result["error"])
	}
	if token == "" {
		return "", "", output.Errorf(output.ExitAPI, "api_error", "upload failed: no file_token returned")
	}
	version := common.GetString(data, "version")
	if version == "" {
		// Some backends return the version under data_version; accept either
		// per the markdown +overwrite contract.
		version = common.GetString(data, "data_version")
	}
	if existingToken != "" && version == "" {
		// The protocol guarantees a non-empty version on overwrite. If the
		// deployed backend hasn't shipped the field yet we surface the gap
		// rather than report a phantom success — callers can downgrade to
		// --if-exists=skip in the meantime.
		return token, "", output.Errorf(output.ExitAPI, "api_error", "overwrite for %q succeeded but no version was returned by upload_all", file.RelPath)
	}
	return token, version, nil
}

func drivePushUploadMultipart(_ context.Context, runtime *common.RuntimeContext, file drivePushLocalFile, existingToken, parentToken string) (string, error) {
	prepareBody := map[string]interface{}{
		"file_name":   file.FileName,
		"parent_type": driveUploadParentTypeExplorer,
		"parent_node": parentToken,
		"size":        file.Size,
	}
	if existingToken != "" {
		prepareBody["file_token"] = existingToken
	}
	prepareResult, err := runtime.CallAPI("POST", "/open-apis/drive/v1/files/upload_prepare", nil, prepareBody)
	if err != nil {
		return "", err
	}

	uploadID := common.GetString(prepareResult, "upload_id")
	blockSize := int64(common.GetFloat(prepareResult, "block_size"))
	blockNum := int(common.GetFloat(prepareResult, "block_num"))
	if uploadID == "" || blockSize <= 0 || blockNum <= 0 {
		return "", output.Errorf(output.ExitAPI, "api_error",
			"upload_prepare returned invalid data: upload_id=%q, block_size=%d, block_num=%d",
			uploadID, blockSize, blockNum)
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Multipart upload: %s, block size %s, %d block(s)\n",
		common.FormatSize(file.Size), common.FormatSize(blockSize), blockNum)

	// Open the local file ONCE for the whole multipart loop. fileio.File
	// implements io.ReaderAt, so each block is a fresh
	// io.NewSectionReader over a shared fd — no need to reopen N times
	// (which is what drive +upload's existing multipart helper does and
	// what the original drive_push copy inherited; that pattern wastes
	// one Open + Close + path-validation per block).
	partFile, err := runtime.FileIO().Open(file.OpenPath)
	if err != nil {
		return "", common.WrapInputStatError(err)
	}
	defer partFile.Close()

	for seq := 0; seq < blockNum; seq++ {
		offset := int64(seq) * blockSize
		partSize := blockSize
		if remaining := file.Size - offset; partSize > remaining {
			partSize = remaining
		}

		fd := larkcore.NewFormdata()
		fd.AddField("upload_id", uploadID)
		fd.AddField("seq", fmt.Sprintf("%d", seq))
		fd.AddField("size", fmt.Sprintf("%d", partSize))
		fd.AddFile("file", io.NewSectionReader(partFile, offset, partSize))

		apiResp, doErr := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodPost,
			ApiPath:    "/open-apis/drive/v1/files/upload_part",
			Body:       fd,
		}, larkcore.WithFileUpload())
		if doErr != nil {
			var exitErr *output.ExitError
			if errors.As(doErr, &exitErr) {
				return "", doErr
			}
			return "", output.ErrNetwork("upload part %d/%d failed: %v", seq+1, blockNum, doErr)
		}

		var partResult map[string]interface{}
		if err := json.Unmarshal(apiResp.RawBody, &partResult); err != nil {
			return "", output.Errorf(output.ExitAPI, "api_error", "upload part %d/%d: invalid response JSON: %v", seq+1, blockNum, err)
		}
		if larkCode := int(common.GetFloat(partResult, "code")); larkCode != 0 {
			msg, _ := partResult["msg"].(string)
			return "", output.ErrAPI(larkCode, fmt.Sprintf("upload part %d/%d failed: [%d] %s", seq+1, blockNum, larkCode, msg), partResult["error"])
		}
		fmt.Fprintf(runtime.IO().ErrOut, "  Block %d/%d uploaded (%s)\n", seq+1, blockNum, common.FormatSize(partSize))
	}

	finishResult, err := runtime.CallAPI("POST", "/open-apis/drive/v1/files/upload_finish", nil, map[string]interface{}{
		"upload_id": uploadID,
		"block_num": blockNum,
	})
	if err != nil {
		return "", err
	}
	token := common.GetString(finishResult, "file_token")
	if token == "" {
		return "", output.Errorf(output.ExitAPI, "api_error", "upload_finish succeeded but no file_token returned")
	}
	return token, nil
}

// drivePushDeleteFile deletes a single Drive file (type=file). Folders are
// never reached here because --delete-remote only iterates the type=file
// subset of the remote listing.
func drivePushDeleteFile(_ context.Context, runtime *common.RuntimeContext, fileToken string) error {
	_, err := runtime.CallAPI(
		"DELETE",
		fmt.Sprintf("/open-apis/drive/v1/files/%s", validate.EncodePathSegment(fileToken)),
		map[string]interface{}{"type": driveTypeFile},
		nil,
	)
	return err
}

// drivePushParentRel returns the parent rel_path of rel ("" when the file
// lives at the root). The local walker emits forward-slash rel_paths so
// path.Dir is the right primitive here, not filepath.Dir.
func drivePushParentRel(rel string) string {
	dir := path.Dir(rel)
	if dir == "." || dir == "/" {
		return ""
	}
	return dir
}

// drivePushSplitRel splits a non-empty rel into (parent, basename), both
// using forward slashes.
func drivePushSplitRel(rel string) (string, string) {
	idx := strings.LastIndex(rel, "/")
	if idx < 0 {
		return "", rel
	}
	return rel[:idx], rel[idx+1:]
}
