// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
	convertlib "github.com/larksuite/cli/shortcuts/im/convert_lib"
)

// imResourceDownloadDir is the fixed sub-directory under the current working
// directory where --download-resources writes fetched resources, so users can
// find and clean them up easily.
const imResourceDownloadDir = "lark-im-resources"

// downloadResourcesFlag opts into automatic resource download. It is shared by
// the three message-listing commands (+chat-messages-list, +messages-mget,
// +threads-messages-list); off by default so the default output contract and
// request count are unchanged.
var downloadResourcesFlag = common.Flag{
	Name: "download-resources",
	Type: "bool",
	Desc: "download image/file/audio/video/media resources (and post-embedded, excluding stickers) into ./lark-im-resources/ (default: off; no extra requests when off)",
}

// downloadResourcesDryRunDesc is the dry-run declaration appended when
// --download-resources is set, mirroring how --no-reactions surfaces the
// reactions enrichment call.
const downloadResourcesDryRunDesc = "Resource download (--download-resources): each downloadable resource (image/file/audio/video/media + post-embedded, excluding stickers) is fetched via GET /open-apis/im/v1/messages/:message_id/resources/:file_key into ./lark-im-resources/ after formatting; deduped by (message_id, file_key) with bounded concurrency, and single-resource failures are isolated."

// resolveResourceDownloadPath builds the safe relative path under
// ./lark-im-resources/ for a resource file_key, reusing
// normalizeDownloadOutputPath so abnormal keys (path separators, traversal,
// absolute paths) are rejected (AC8).
func resolveResourceDownloadPath(fileKey string) (string, error) {
	return normalizeDownloadOutputPath(fileKey, imResourceDownloadDir+"/"+fileKey)
}

// enrichMessageResourceDownloads downloads the resource refs extracted during
// formatting into ./lark-im-resources/, filling local_path/size_bytes back into
// each message's resources block. The download engine isolates single-resource
// failures (fail-silent + stderr warning), so the main message output is never
// blocked.
func enrichMessageResourceDownloads(runtime *common.RuntimeContext, messages []map[string]interface{}) {
	convertlib.EnrichResourceDownloads(runtime, messages, func(dlCtx context.Context, messageID, key, fileType string) (string, int64, error) {
		rel, err := resolveResourceDownloadPath(key)
		if err != nil {
			return "", 0, err
		}
		if _, err := runtime.ResolveSavePath(rel); err != nil {
			return "", 0, err
		}
		// preserveBasename=true: each resource is keyed by its unique file_key,
		// so keep that basename and only append an extension. Adopting the
		// server's Content-Disposition filename here would let two resources
		// that share a filename (e.g. download.bin) clobber each other.
		return downloadIMResourceToPath(dlCtx, runtime, messageID, key, fileType, rel, true)
	})
}
