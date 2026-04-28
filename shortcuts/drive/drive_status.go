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

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	driveStatusListPageSize = 200
	driveStatusFileType     = "file"
	driveStatusFolderType   = "folder"
)

type driveStatusEntry struct {
	RelPath   string `json:"rel_path"`
	FileToken string `json:"file_token,omitempty"`
}

// DriveStatus walks --local-dir, recursively lists --folder-token, and reports
// four buckets (new_local, new_remote, modified, unchanged) by SHA-256 hash.
//
// Only Drive entries with type=file are compared; online docs (docx, sheet,
// bitable, mindnote, slides) and shortcuts are skipped because there is no
// equivalent local binary to hash against.
//
// SafeInputPath (applied by runtime.FileIO()) rejects absolute paths and any
// path that resolves outside cwd, which keeps the local side bounded to the
// caller's working directory.
var DriveStatus = common.Shortcut{
	Service:     "drive",
	Command:     "+status",
	Description: "Compare a local directory with a Drive folder by content hash",
	Risk:        "read",
	Scopes:      []string{"drive:drive.metadata:readonly", "drive:file:download"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "local-dir", Desc: "local root directory (relative to cwd)", Required: true},
		{Name: "folder-token", Desc: "Drive folder token", Required: true},
	},
	Tips: []string{
		"Only entries with type=file are compared; online docs (docx, sheet, bitable, mindnote, slides) and shortcuts are skipped.",
		"Files present on both sides are downloaded and SHA-256 hashed in memory to decide modified vs unchanged; expect noticeable I/O on large folders.",
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
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			Desc("Walk --local-dir, recursively list --folder-token, and download files present on both sides to compare SHA-256.").
			GET("/open-apis/drive/v1/files").
			Set("folder_token", runtime.Str("folder-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		localDir := strings.TrimSpace(runtime.Str("local-dir"))
		folderToken := strings.TrimSpace(runtime.Str("folder-token"))

		fmt.Fprintf(runtime.IO().ErrOut, "Walking local: %s\n", localDir)
		localHashes, err := walkLocalForStatus(runtime, localDir)
		if err != nil {
			return err
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Listing Drive folder: %s\n", common.MaskToken(folderToken))
		remoteFiles, err := listRemoteForStatus(ctx, runtime, folderToken, "")
		if err != nil {
			return err
		}

		paths := mergeStatusPaths(localHashes, remoteFiles)

		var newLocal, newRemote, modified, unchanged []driveStatusEntry
		for _, relPath := range paths {
			localHash, hasLocal := localHashes[relPath]
			remoteToken, hasRemote := remoteFiles[relPath]
			switch {
			case hasLocal && !hasRemote:
				newLocal = append(newLocal, driveStatusEntry{RelPath: relPath})
			case !hasLocal && hasRemote:
				newRemote = append(newRemote, driveStatusEntry{RelPath: relPath, FileToken: remoteToken})
			default:
				remoteHash, err := hashRemoteForStatus(ctx, runtime, remoteToken)
				if err != nil {
					return err
				}
				entry := driveStatusEntry{RelPath: relPath, FileToken: remoteToken}
				if localHash == remoteHash {
					unchanged = append(unchanged, entry)
				} else {
					modified = append(modified, entry)
				}
			}
		}

		runtime.Out(map[string]interface{}{
			"new_local":  emptyIfNil(newLocal),
			"new_remote": emptyIfNil(newRemote),
			"modified":   emptyIfNil(modified),
			"unchanged":  emptyIfNil(unchanged),
		}, nil)
		return nil
	},
}

func walkLocalForStatus(runtime *common.RuntimeContext, root string) (map[string]string, error) {
	files := make(map[string]string)
	// FileIO has no walker today and shortcuts can't import internal/vfs;
	// SafeInputPath (via runtime.FileIO().Stat above) has already bounded
	// root inside cwd, so a vanilla filepath.WalkDir is acceptable here.
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error { //nolint:forbidigo // see comment above
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum, err := hashLocalForStatus(runtime, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = sum
		return nil
	})
	if err != nil {
		return nil, output.Errorf(output.ExitInternal, "io", "walk %s: %s", root, err)
	}
	return files, nil
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

func listRemoteForStatus(ctx context.Context, runtime *common.RuntimeContext, folderToken, relBase string) (map[string]string, error) {
	files := make(map[string]string)
	pageToken := ""
	for {
		params := map[string]interface{}{
			"folder_token": folderToken,
			"page_size":    fmt.Sprint(driveStatusListPageSize),
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
			case driveStatusFileType:
				files[joinRelStatus(relBase, fName)] = fToken
			case driveStatusFolderType:
				subFiles, err := listRemoteForStatus(ctx, runtime, fToken, joinRelStatus(relBase, fName))
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

func joinRelStatus(base, name string) string {
	if base == "" {
		return name
	}
	return base + "/" + name
}

func mergeStatusPaths(local, remote map[string]string) []string {
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
