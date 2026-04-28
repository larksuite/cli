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
	drivePullListPageSize      = 200
	drivePullFileType          = "file"
	drivePullFolderType        = "folder"
)

type drivePullItem struct {
	RelPath   string `json:"rel_path"`
	FileToken string `json:"file_token,omitempty"`
	Action    string `json:"action"`
	Error     string `json:"error,omitempty"`
}

// DrivePull mirrors a Drive folder onto a local directory: recursively lists
// --folder-token, downloads each type=file entry under --local-dir, and
// optionally deletes local files absent from Drive (--delete-local --yes).
//
// Only Drive entries with type=file participate; online docs (docx, sheet,
// bitable, mindnote, slides) and shortcuts are skipped because there is no
// equivalent local binary to write back.
var DrivePull = common.Shortcut{
	Service:     "drive",
	Command:     "+pull",
	Description: "Mirror a Drive folder onto a local directory (Drive → local)",
	Risk:        "write",
	Scopes:      []string{"drive:drive.metadata:readonly", "drive:file:download"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "local-dir", Desc: "local root directory (relative to cwd)", Required: true},
		{Name: "folder-token", Desc: "source Drive folder token", Required: true},
		{Name: "if-exists", Desc: "policy when a local file already exists", Default: drivePullIfExistsOverwrite, Enum: []string{drivePullIfExistsOverwrite, drivePullIfExistsSkip}},
		{Name: "delete-local", Type: "bool", Desc: "delete local files absent from Drive (mirror semantics); requires --yes"},
		{Name: "yes", Type: "bool", Desc: "confirm --delete-local before deleting local files"},
	},
	Tips: []string{
		"Only entries with type=file are downloaded; online docs (docx, sheet, bitable, mindnote, slides) and shortcuts are skipped.",
		"Subfolders recurse and are reproduced as local directories under --local-dir; missing parents are created automatically.",
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
		remoteFiles, err := drivePullListRemote(ctx, runtime, folderToken, "")
		if err != nil {
			return err
		}

		var downloaded, skipped, failed, deletedLocal int
		items := make([]drivePullItem, 0)

		// Deterministic iteration order for output stability.
		remotePaths := make([]string, 0, len(remoteFiles))
		for p := range remoteFiles {
			remotePaths = append(remotePaths, p)
		}
		sort.Strings(remotePaths)

		for _, rel := range remotePaths {
			token := remoteFiles[rel]
			target := filepath.Join(rootRelToCwd, rel)

			if _, statErr := runtime.FileIO().Stat(target); statErr == nil {
				if ifExists == drivePullIfExistsSkip {
					items = append(items, drivePullItem{RelPath: rel, FileToken: token, Action: "skipped"})
					skipped++
					continue
				}
			}

			if err := drivePullDownload(ctx, runtime, token, target); err != nil {
				items = append(items, drivePullItem{RelPath: rel, FileToken: token, Action: "failed", Error: err.Error()})
				failed++
				continue
			}
			items = append(items, drivePullItem{RelPath: rel, FileToken: token, Action: "downloaded"})
			downloaded++
		}

		if deleteLocal {
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
					continue
				}
				rel = filepath.ToSlash(rel)
				if _, ok := remoteFiles[rel]; ok {
					continue
				}
				// FileIO has no Remove(); the absolute path comes from
				// walking safeRoot, which validate.SafeInputPath has
				// already bounded inside cwd, so a bare os.Remove is
				// acceptable here.
				if err := os.Remove(absPath); err != nil { //nolint:forbidigo // see comment above
					items = append(items, drivePullItem{RelPath: rel, Action: "delete_failed", Error: err.Error()})
					continue
				}
				items = append(items, drivePullItem{RelPath: rel, Action: "deleted_local"})
				deletedLocal++
			}
		}

		runtime.Out(map[string]interface{}{
			"summary": map[string]interface{}{
				"downloaded":    downloaded,
				"skipped":       skipped,
				"failed":        failed,
				"deleted_local": deletedLocal,
			},
			"items": items,
		}, nil)
		return nil
	},
}

// drivePullListRemote recursively lists a Drive folder and returns a map of
// rel_path → file_token for entries with type=file. Subfolders recurse;
// online docs and shortcuts are skipped (no equivalent local binary).
//
// TODO(post-#692): when drive +status merges, lift this and the matching
// helper in drive_status.go into a shared listRemoteFolderFiles in the
// drive package.
func drivePullListRemote(ctx context.Context, runtime *common.RuntimeContext, folderToken, relBase string) (map[string]string, error) {
	files := make(map[string]string)
	pageToken := ""
	for {
		params := map[string]interface{}{
			"folder_token": folderToken,
			"page_size":    fmt.Sprint(drivePullListPageSize),
		}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		result, err := runtime.CallAPI("GET", "/open-apis/drive/v1/files", params, nil)
		if err != nil {
			return nil, err
		}
		rawFiles, _ := result["files"].([]interface{})
		for _, item := range rawFiles {
			f, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			fType := common.GetString(f, "type")
			fName := common.GetString(f, "name")
			fToken := common.GetString(f, "token")
			if fName == "" || fToken == "" {
				continue
			}
			switch fType {
			case drivePullFileType:
				files[drivePullJoinRel(relBase, fName)] = fToken
			case drivePullFolderType:
				subFiles, err := drivePullListRemote(ctx, runtime, fToken, drivePullJoinRel(relBase, fName))
				if err != nil {
					return nil, err
				}
				for k, v := range subFiles {
					files[k] = v
				}
			}
		}
		hasMore, _ := result["has_more"].(bool)
		nextToken := common.GetString(result, "next_page_token")
		if !hasMore || nextToken == "" {
			break
		}
		pageToken = nextToken
	}
	return files, nil
}

func drivePullJoinRel(base, name string) string {
	if base == "" {
		return name
	}
	return base + "/" + name
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

// drivePullWalkLocal walks the canonical absolute root and returns the
// absolute paths of every regular file underneath it. The caller deletes
// some of these paths, so it is critical that they are produced by
// walking a canonical root (no symlinks in the path) — otherwise OS path
// resolution could redirect a delete to a file outside cwd. Same threat
// model as drive_status.go.
func drivePullWalkLocal(root string) ([]string, error) {
	var paths []string
	// FileIO has no walker today; the root passed in is the canonical
	// absolute path from validate.SafeInputPath, so WalkDir's default
	// "do not follow child symlinks" policy keeps the traversal inside
	// the validated subtree.
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
