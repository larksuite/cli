// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strconv"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	driveListRemotePageSize = 200
	driveTypeFile           = "file"
	driveTypeFolder         = "folder"
	driveUniqueSuffixMaxSeq = 1024
)

// driveRemoteEntry is one Drive entry returned by listRemoteFolderEntries. It
// carries enough metadata for every shortcut that consumes the listing
// to build its own per-shortcut view by filtering on Type.
type driveRemoteEntry struct {
	// FileToken is the Drive token for this entry. For type=folder this
	// is the folder_token; for everything else it is the file_token.
	FileToken string
	Name      string
	Size      int64
	// Type is the Drive entry kind verbatim from the API:
	// "file" | "folder" | "docx" | "doc" | "sheet" | "bitable" |
	// "mindnote" | "slides" | "shortcut" | …
	Type         string
	CreatedTime  string
	ModifiedTime string
	// RelPath is the entry's path relative to the listing root. Encoded
	// with "/" separators on every platform so it matches the rel_paths
	// produced by the shortcuts' local walkers.
	RelPath string
}

type driveDuplicateRemoteEntry struct {
	FileToken    string `json:"file_token"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Size         int64  `json:"size,omitempty"`
	CreatedTime  string `json:"created_time,omitempty"`
	ModifiedTime string `json:"modified_time,omitempty"`
}

type driveDuplicateRemotePath struct {
	RelPath string                      `json:"rel_path"`
	Entries []driveDuplicateRemoteEntry `json:"entries"`
}

// listRemoteFolderEntries recursively lists folderToken under relBase and
// returns one entry per Drive item. Subfolders are descended into and the
// folder's own entry is also recorded, allowing callers to detect multiple
// remote files that map to the same rel_path.
//
// The helper deliberately stores every Drive object kind. Online docs and
// shortcuts are skipped by sync shortcuts later, but preserving their rel_path
// here prevents destructive mirror modes from treating a local same-named
// regular file as an orphan when Drive already owns that path.
//
// Pagination uses common.PaginationMeta, which accepts both page_token and
// next_page_token.
func listRemoteFolderEntries(ctx context.Context, runtime *common.RuntimeContext, folderToken, relBase string) ([]driveRemoteEntry, error) {
	var out []driveRemoteEntry
	pageToken := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
			out = append(out, driveRemoteEntry{
				FileToken:    fToken,
				Name:         fName,
				Size:         int64(common.GetFloat(f, "size")),
				Type:         fType,
				CreatedTime:  common.GetString(f, "created_time"),
				ModifiedTime: common.GetString(f, "modified_time"),
				RelPath:      rel,
			})
			if fType == driveTypeFolder {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				sub, err := listRemoteFolderEntries(ctx, runtime, fToken, rel)
				if err != nil {
					return nil, err
				}
				out = append(out, sub...)
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

func duplicateRemoteFilePaths(entries []driveRemoteEntry) []driveDuplicateRemotePath {
	groups := make(map[string][]driveRemoteEntry)
	for _, entry := range entries {
		groups[entry.RelPath] = append(groups[entry.RelPath], entry)
	}

	relPaths := make([]string, 0, len(groups))
	for relPath, grouped := range groups {
		if len(grouped) > 1 {
			relPaths = append(relPaths, relPath)
		}
	}
	sort.Strings(relPaths)

	duplicates := make([]driveDuplicateRemotePath, 0, len(relPaths))
	for _, relPath := range relPaths {
		grouped := append([]driveRemoteEntry(nil), groups[relPath]...)
		sort.SliceStable(grouped, func(i, j int) bool {
			if grouped[i].Type != grouped[j].Type {
				return grouped[i].Type < grouped[j].Type
			}
			if cmp, ok := compareDriveTimes(grouped[i].CreatedTime, grouped[j].CreatedTime); ok && cmp != 0 {
				return cmp < 0
			}
			if cmp, ok := compareDriveTimes(grouped[i].ModifiedTime, grouped[j].ModifiedTime); ok && cmp != 0 {
				return cmp < 0
			}
			return grouped[i].FileToken < grouped[j].FileToken
		})
		dupEntries := make([]driveDuplicateRemoteEntry, 0, len(grouped))
		for _, entry := range grouped {
			dupEntries = append(dupEntries, driveDuplicateRemoteEntry{
				FileToken:    entry.FileToken,
				Name:         entry.Name,
				Type:         entry.Type,
				Size:         entry.Size,
				CreatedTime:  entry.CreatedTime,
				ModifiedTime: entry.ModifiedTime,
			})
		}
		duplicates = append(duplicates, driveDuplicateRemotePath{RelPath: relPath, Entries: dupEntries})
	}
	return duplicates
}

func duplicateRemotePathError(duplicates []driveDuplicateRemotePath) *output.ExitError {
	return &output.ExitError{
		Code: output.ExitAPI,
		Detail: &output.ErrDetail{
			Type:    "duplicate_remote_path",
			Message: "multiple Drive entries map to the same rel_path",
			Detail: map[string]interface{}{
				"duplicates_remote": duplicates,
			},
		},
	}
}

const (
	driveDuplicateRemoteFail   = "fail"
	driveDuplicateRemoteRename = "rename"
	driveDuplicateRemoteNewest = "newest"
	driveDuplicateRemoteOldest = "oldest"
)

// sortRemoteFiles orders duplicate Drive files according to the conflict
// strategy, using parsed Drive timestamps so mixed second/millisecond/
// microsecond epochs compare by actual time rather than raw integer width.
func sortRemoteFiles(files []driveRemoteEntry, strategy string) {
	sort.SliceStable(files, func(i, j int) bool {
		a, b := files[i], files[j]
		switch strategy {
		case driveDuplicateRemoteNewest:
			if cmp, ok := compareDriveTimes(a.ModifiedTime, b.ModifiedTime); ok && cmp != 0 {
				return cmp > 0
			} else if !ok {
				return a.FileToken < b.FileToken
			}
			if cmp, ok := compareDriveTimes(a.CreatedTime, b.CreatedTime); ok && cmp != 0 {
				return cmp > 0
			} else if !ok {
				return a.FileToken < b.FileToken
			}
		default:
			if cmp, ok := compareDriveTimes(a.CreatedTime, b.CreatedTime); ok && cmp != 0 {
				return cmp < 0
			} else if !ok {
				return a.FileToken < b.FileToken
			}
			if cmp, ok := compareDriveTimes(a.ModifiedTime, b.ModifiedTime); ok && cmp != 0 {
				return cmp < 0
			} else if !ok {
				return a.FileToken < b.FileToken
			}
		}
		return a.FileToken < b.FileToken
	})
}

// compareDriveTimes compares two Drive epoch strings after normalizing their
// unit (seconds, milliseconds, or microseconds) into time.Time values.
func compareDriveTimes(a, b string) (int, bool) {
	at, _, aOK := parseDriveEpoch(a)
	bt, _, bOK := parseDriveEpoch(b)
	if !aOK || !bOK {
		return 0, false
	}
	switch {
	case at.Before(bt):
		return -1, true
	case at.After(bt):
		return 1, true
	default:
		return 0, true
	}
}

func parseDriveEpoch(raw string) (time.Time, time.Duration, bool) {
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	// Drive timestamps are epoch strings. The API currently returns
	// milliseconds, but tests and older payloads may still use seconds.
	// Infer the unit conservatively from magnitude and compare local mtimes
	// at the same resolution so sub-second filesystem noise does not force
	// a transfer in smart mode.
	switch {
	case v > 1e14 || v < -1e14:
		return time.UnixMicro(v), time.Microsecond, true
	case v > 1e11 || v < -1e11:
		return time.UnixMilli(v), time.Millisecond, true
	default:
		return time.Unix(v, 0), time.Second, true
	}
}

// compareDriveRemoteModifiedToLocal compares one Drive modified_time string to a
// local file mtime.
//   - returns -1 when remote < local
//   - returns  0 when remote == local at the remote timestamp resolution
//   - returns  1 when remote > local
//
// The bool reports whether the remote timestamp was parseable.
func compareDriveRemoteModifiedToLocal(remoteModified string, local time.Time) (int, bool) {
	remoteTime, resolution, ok := parseDriveEpoch(remoteModified)
	if !ok {
		return 0, false
	}
	localAtRemoteResolution := local.Truncate(resolution)
	switch {
	case remoteTime.Before(localAtRemoteResolution):
		return -1, true
	case remoteTime.After(localAtRemoteResolution):
		return 1, true
	default:
		return 0, true
	}
}

func chooseRemoteFile(files []driveRemoteEntry, strategy string) (driveRemoteEntry, error) {
	if len(files) == 0 {
		return driveRemoteEntry{}, fmt.Errorf("no Drive entries available for strategy %q", strategy)
	}
	candidates := append([]driveRemoteEntry(nil), files...)
	sortRemoteFiles(candidates, strategy)
	return candidates[0], nil
}

func isFileOnlyDuplicatePath(duplicate driveDuplicateRemotePath) bool {
	if len(duplicate.Entries) < 2 {
		return false
	}
	for _, entry := range duplicate.Entries {
		if entry.Type != driveTypeFile {
			return false
		}
	}
	return true
}

func blockingRemotePathConflicts(entries []driveRemoteEntry, duplicateRemote string) []driveDuplicateRemotePath {
	duplicates := duplicateRemoteFilePaths(entries)
	if duplicateRemote == driveDuplicateRemoteFail {
		return duplicates
	}
	blocking := make([]driveDuplicateRemotePath, 0, len(duplicates))
	for _, duplicate := range duplicates {
		if !isFileOnlyDuplicatePath(duplicate) {
			blocking = append(blocking, duplicate)
		}
	}
	return blocking
}

func occupiedRemotePaths(entries []driveRemoteEntry) map[string]struct{} {
	occupied := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		occupied[entry.RelPath] = struct{}{}
	}
	return occupied
}

func stableTokenHash(fileToken string) string {
	sum := sha256.Sum256([]byte(fileToken))
	return hex.EncodeToString(sum[:])
}

func stableTokenIdentifier(fileToken string) string {
	hash := stableTokenHash(fileToken)
	if len(hash) > 12 {
		hash = hash[:12]
	}
	return "hash_" + hash
}

func relPathWithSuffix(relPath, suffix string) string {
	dir, base := path.Split(relPath)
	ext := path.Ext(base)
	if ext == base {
		return dir + base + suffix
	}
	stem := base[:len(base)-len(ext)]
	return dir + stem + suffix + ext
}

func relPathWithUniqueFileTokenSuffix(relPath, fileToken string, occupied map[string]struct{}) (string, error) {
	tokenHash := stableTokenHash(fileToken)
	suffixes := []string{
		"__lark_" + tokenHash[:12],
		"__lark_" + tokenHash[:24],
		"__lark_" + tokenHash,
	}
	for _, suffix := range suffixes {
		candidate := relPathWithSuffix(relPath, suffix)
		if _, exists := occupied[candidate]; !exists {
			occupied[candidate] = struct{}{}
			return candidate, nil
		}
	}
	for attempt := 2; attempt <= driveUniqueSuffixMaxSeq; attempt++ {
		candidate := relPathWithSuffix(relPath, "__lark_"+tokenHash+"_"+strconv.Itoa(attempt))
		if _, exists := occupied[candidate]; !exists {
			occupied[candidate] = struct{}{}
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not generate a unique rel_path for %q after %d attempts", relPath, driveUniqueSuffixMaxSeq)
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
