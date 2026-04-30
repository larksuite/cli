// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/shortcuts/common"
)

const (
	driveListRemotePageSize = 200
	driveTypeFile           = "file"
	driveTypeFolder         = "folder"
)

// driveRemoteEntry is one Drive entry returned by listRemoteFolder. It
// carries enough metadata for every shortcut that consumes the listing
// to build its own per-shortcut view by filtering on Type.
type driveRemoteEntry struct {
	// FileToken is the Drive token for this entry. For type=folder this
	// is the folder_token; for everything else it is the file_token.
	FileToken string
	// Type is the Drive entry kind verbatim from the API:
	// "file" | "folder" | "docx" | "doc" | "sheet" | "bitable" |
	// "mindnote" | "slides" | "shortcut" | …
	Type string
	// RelPath is the entry's path relative to the listing root. Encoded
	// with "/" separators on every platform so it matches the rel_paths
	// produced by the shortcuts' local walkers.
	RelPath string
}

// listRemoteFolder recursively lists folderToken under relBase and
// returns one entry per Drive item, keyed by rel_path. Subfolders are
// descended into and the folder's own entry is also recorded — callers
// can reason about "this rel_path is occupied by a folder" without
// re-listing.
//
// This is the shared backbone for the three sync-disk shortcuts. None
// of them need every field at every call site, so each one filters
// on Type:
//
//   - +status (drive_status.go) keeps Type=="file" and uses FileToken
//     to drive content-hash diffs against the local tree.
//   - +pull (drive_pull.go) keeps Type=="file" + FileToken for the
//     download set, and the full key set (every rel_path) as the
//     guard for --delete-local.
//   - +push (drive_push.go) keeps Type=="file" + FileToken for upload /
//     overwrite / orphan-delete decisions, and Type=="folder" + FileToken
//     for the create_folder cache.
//
// Pagination uses common.PaginationMeta, which accepts both
// page_token and next_page_token — the Drive list endpoint has
// historically returned the latter, but the helper future-proofs
// against a backend rename.
func listRemoteFolder(ctx context.Context, runtime *common.RuntimeContext, folderToken, relBase string) (map[string]driveRemoteEntry, error) {
	out := make(map[string]driveRemoteEntry)
	pageToken := ""
	for {
		params := map[string]interface{}{
			"folder_token": folderToken,
			"page_size":    fmt.Sprint(driveListRemotePageSize),
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
			rel := joinRelDrive(relBase, fName)
			out[rel] = driveRemoteEntry{FileToken: fToken, Type: fType, RelPath: rel}
			if fType == driveTypeFolder {
				sub, err := listRemoteFolder(ctx, runtime, fToken, rel)
				if err != nil {
					return nil, err
				}
				for k, v := range sub {
					out[k] = v
				}
			}
		}
		hasMore, nextToken := common.PaginationMeta(result)
		if !hasMore || nextToken == "" {
			break
		}
		pageToken = nextToken
	}
	return out, nil
}

// joinRelDrive joins a rel_path base with an entry name using "/".
// Empty base means the entry sits at the listing root. Mirrors the
// behavior the per-shortcut helpers used to ship and keeps rel_paths
// stable across +status / +pull / +push.
func joinRelDrive(base, name string) string {
	if base == "" {
		return name
	}
	return base + "/" + name
}
