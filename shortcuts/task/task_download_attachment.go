// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/download"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const taskAttachmentGetPath = "/open-apis/task/v2/attachments/%s"

type taskAttachmentDownloadMetadata struct {
	GUID      string `json:"guid"`
	FileToken string `json:"file_token"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	URL       string `json:"url"`
}

// DownloadAttachmentTask downloads one Task attachment through the short-lived
// URL returned by the attachment detail endpoint.
var DownloadAttachmentTask = common.Shortcut{
	Service:     "task",
	Command:     "+download-attachment",
	Description: "download a task attachment by attachment GUID",
	Risk:        "read",
	Scopes:      []string{"task:attachment:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "attachment-guid", Desc: "attachment guid returned by a Task attachment API", Required: true},
		{Name: "output", Desc: "relative local save path; an existing directory or a path ending in / uses the attachment name", Required: true},
		{Name: "overwrite", Type: "bool", Desc: "overwrite an existing output file"},
		{Name: "user-id-type", Desc: "user id type (default: open_id)", Default: "open_id"},
	},

	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		guid := strings.TrimSpace(runtime.Str("attachment-guid"))
		userIDType := taskAttachmentUserIDType(runtime)
		return common.NewDryRunAPI().
			GET(fmt.Sprintf(taskAttachmentGetPath, validate.EncodePathSegment(guid))).
			Desc("[1] Fetch attachment metadata and a fresh temporary download URL").
			Params(map[string]interface{}{"user_id_type": userIDType}).
			GET("<temporary_attachment_url>").
			Desc("[2] Download the attachment without a Lark Authorization header").
			Set("output", runtime.Str("output"))
	},

	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("attachment-guid")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "attachment guid cannot be empty").WithParam("--attachment-guid")
		}
		output := strings.TrimSpace(runtime.Str("output"))
		if output == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "output path cannot be empty").WithParam("--output")
		}
		if _, err := runtime.ResolveSavePath(output); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).
				WithParam("--output").
				WithCause(err)
		}
		return nil
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		guid := strings.TrimSpace(runtime.Str("attachment-guid"))
		metadata, err := fetchTaskAttachmentDownloadMetadata(runtime, guid, taskAttachmentUserIDType(runtime))
		if err != nil {
			return err
		}

		targetPath, err := taskAttachmentTargetPath(runtime, runtime.Str("output"), metadata.Name, runtime.Bool("overwrite"))
		if err != nil {
			return err
		}

		stream, err := openTaskAttachmentDownload(ctx, runtime, metadata.URL)
		if taskAttachmentURLNeedsRefresh(err) {
			metadata, err = fetchTaskAttachmentDownloadMetadata(runtime, guid, taskAttachmentUserIDType(runtime))
			if err == nil {
				stream, err = openTaskAttachmentDownload(ctx, runtime, metadata.URL)
			}
		}
		if err != nil {
			return err
		}
		defer stream.Body.Close()

		if metadata.Size > 0 && stream.ContentLength >= 0 && stream.ContentLength != metadata.Size {
			return errs.NewNetworkError(
				errs.SubtypeNetworkRepresentationChanged,
				"attachment size changed before download: metadata reports %d bytes, response reports %d",
				metadata.Size,
				stream.ContentLength,
			).WithRetryable().WithHint("retry the command to fetch fresh attachment metadata")
		}

		saved, err := runtime.FileIO().Save(targetPath, fileio.SaveOptions{
			ContentType:   stream.Header.Get("Content-Type"),
			ContentLength: stream.ContentLength,
		}, stream.Body)
		if err != nil {
			return common.WrapSaveErrorTypedForFlag(err, "--output")
		}

		savedPath, resolveErr := runtime.ResolveSavePath(targetPath)
		if resolveErr != nil || savedPath == "" {
			savedPath = targetPath
		}
		result := map[string]interface{}{
			"attachment_guid": metadata.GUID,
			"file_token":      metadata.FileToken,
			"name":            metadata.Name,
			"saved_path":      savedPath,
			"size_bytes":      saved.Size(),
			"content_type":    stream.Header.Get("Content-Type"),
		}
		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✓ Downloaded attachment %s → %s (%s)\n", metadata.Name, savedPath, common.FormatSize(saved.Size()))
		})
		return nil
	},
}

func taskAttachmentUserIDType(runtime *common.RuntimeContext) string {
	value := strings.TrimSpace(runtime.Str("user-id-type"))
	if value == "" {
		return "open_id"
	}
	return value
}

func fetchTaskAttachmentDownloadMetadata(runtime *common.RuntimeContext, guid, userIDType string) (taskAttachmentDownloadMetadata, error) {
	data, err := runtime.CallAPITyped(
		http.MethodGet,
		fmt.Sprintf(taskAttachmentGetPath, validate.EncodePathSegment(guid)),
		map[string]interface{}{"user_id_type": userIDType},
		nil,
	)
	if err != nil {
		return taskAttachmentDownloadMetadata{}, err
	}

	raw, ok := data["attachment"]
	if !ok {
		return taskAttachmentDownloadMetadata{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "attachment response has no attachment object")
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return taskAttachmentDownloadMetadata{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "encode attachment metadata: %s", err).WithCause(err)
	}
	var metadata taskAttachmentDownloadMetadata
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		return taskAttachmentDownloadMetadata{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "decode attachment metadata: %s", err).WithCause(err)
	}
	if strings.TrimSpace(metadata.GUID) == "" {
		return taskAttachmentDownloadMetadata{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "attachment response has no attachment guid")
	}
	if strings.TrimSpace(metadata.URL) == "" {
		return taskAttachmentDownloadMetadata{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "attachment response has no temporary download URL")
	}
	return metadata, nil
}

func taskAttachmentTargetPath(runtime *common.RuntimeContext, output, attachmentName string, overwrite bool) (string, error) {
	output = strings.TrimSpace(output)
	outputIsDir := strings.HasSuffix(output, "/") || strings.HasSuffix(output, string(filepath.Separator))
	if info, err := runtime.FileIO().Stat(output); err == nil {
		outputIsDir = info.IsDir()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", taskAttachmentStatError(err)
	}

	targetPath := output
	if outputIsDir {
		targetPath = filepath.Join(output, taskAttachmentFileName(attachmentName))
	}
	if _, err := runtime.ResolveSavePath(targetPath); err != nil {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).
			WithParam("--output").
			WithCause(err)
	}
	if !overwrite {
		if _, err := runtime.FileIO().Stat(targetPath); err == nil {
			return "", errs.NewValidationError(errs.SubtypeAlreadyExists, "output file already exists: %s", targetPath).
				WithParam("--output").
				WithHint("use --overwrite to replace the existing file")
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", taskAttachmentStatError(err)
		}
	}
	return targetPath, nil
}

func taskAttachmentStatError(err error) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot inspect output path: %s", err).
			WithParam("--output").
			WithCause(err)
	}
	return errs.NewInternalError(errs.SubtypeFileIO, "cannot inspect output path: %s", err).WithCause(err)
}

func taskAttachmentFileName(name string) string {
	name = strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	name = path.Base(name)
	if name == "" || name == "." || name == ".." || strings.ContainsFunc(name, func(r rune) bool {
		return r < 0x20 || r == 0x7f
	}) {
		return "attachment"
	}
	return name
}

func openTaskAttachmentDownload(ctx context.Context, runtime *common.RuntimeContext, rawURL string) (*download.Stream, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "attachment response contains an invalid download URL").WithCause(err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" || parsed.User != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "attachment download URL must be an HTTPS URL without user information")
	}
	httpClient, err := runtime.Factory.ExternalHTTPClient()
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError, "failed to get external HTTP client: %s", err).WithCause(err)
	}
	return download.Open(
		ctx,
		download.ImmutableSource(download.OpaqueURL(httpClient, rawURL)),
		download.Options{
			DisableMultipart: true,
			MaxPartRetries:   1,
		},
	)
}

func taskAttachmentURLNeedsRefresh(err error) bool {
	problem, ok := errs.ProblemOf(err)
	return ok && (problem.Code == http.StatusUnauthorized || problem.Code == http.StatusForbidden)
}
