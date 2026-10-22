// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"errors"
	"io"

	"github.com/larksuite/cli/errs"
	extdownload "github.com/larksuite/cli/extension/download"
	"github.com/larksuite/cli/internal/downloadtransport"
	"github.com/larksuite/cli/shortcuts/common"
)

// openDriveFileDownload streams the file in validated ranged parts.
//
// The file token does not pin the bytes: +upload --file-token replaces the
// content in place, and the download response carries no ETag. Parts may be
// combined only when the caller holds the latest_modify_time read before the
// first part and re-checks it after the last (driveRevisionCheckReader).
// Without it, MutableSource makes the engine fall back to one full response.
//
// Ceiling: latest_modify_time has one-second resolution, so a replacement that
// lands in the same second as the revision read is not detected. A server
// validator (ETag or a readable version) would close that window.
func openDriveFileDownload(ctx context.Context, runtime *common.RuntimeContext, fileToken, revision string) (*extdownload.Stream, error) {
	transport := downloadtransport.NewOAPI(runtime.DoAPIStream).Get(
		"/open-apis/drive/v1/files/:file_token/download",
		downloadtransport.PathParam("file_token", fileToken),
	)
	source := extdownload.MutableSource(transport)
	if revision != "" {
		source = extdownload.ImmutableSource(transport)
	}
	return extdownload.Open(ctx, source, extdownload.Options{PartSize: driveDownloadPartSize})
}

// driveRevisionCheckReader runs verify before reporting EOF, so a file that
// changed during a multipart download fails before the output is committed.
type driveRevisionCheckReader struct {
	reader io.Reader
	verify func() error
	err    error
}

func (r *driveRevisionCheckReader) Read(p []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	n, err := r.reader.Read(p)
	if errors.Is(err, io.EOF) {
		if verifyErr := r.verify(); verifyErr != nil {
			r.err = verifyErr
			return n, verifyErr
		}
	}
	return n, err
}

func verifyDriveDownloadRevision(runtime *common.RuntimeContext, fileToken, revision string) error {
	meta, err := common.FetchDriveMeta(runtime, fileToken, "file", false)
	if err != nil {
		return wrapDriveNetworkErr(err, "cannot confirm the file was unchanged during a multipart download: %s", err)
	}
	if meta.LatestModifyTime != revision {
		return errs.NewNetworkError(errs.SubtypeNetworkRepresentationChanged,
			"file changed while downloading: latest_modify_time is %q, want %q", meta.LatestModifyTime, revision).
			WithRetryable().
			WithHint("run the command again to download the current version")
	}
	return nil
}

// driveDownloadRevisionBody validates multipart data before Save sees EOF.
func driveDownloadRevisionBody(runtime *common.RuntimeContext, token, revision string, stream *extdownload.Stream) io.Reader {
	if revision == "" || stream.ContentLength <= driveDownloadPartSize {
		return stream.Body
	}
	return &driveRevisionCheckReader{reader: stream.Body, verify: func() error {
		return verifyDriveDownloadRevision(runtime, token, revision)
	}}
}
