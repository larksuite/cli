// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image/jpeg"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	vcMeetingScreenshotAPIPath = "/open-apis/vc/v1/bots/screenshot"
	meetingScreenshotMaxBytes  = 8 << 20
)

var meetingScreenshotNow = time.Now

type meetingScreenshotRequest struct {
	MeetingID string `json:"meeting_id"`
}

// VCMeetingScreenshot captures a video meeting screenshot and writes the
// returned JPEG locally.
var VCMeetingScreenshot = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-screenshot",
	Description: "Capture a video meeting screenshot",
	Risk:        "read",
	Scopes:      []string{"vc:meeting.realtime:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "meeting-id", Required: true, Desc: "long numeric meeting ID (not the 9-digit meeting number)"},
		{Name: "output", Desc: "JPEG output path relative to the current working directory; parent directories are created"},
		{Name: "overwrite", Type: "bool", Desc: "replace the output file if it already exists"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateMeetingEventsMeetingID(runtime.Str("meeting-id")); err != nil {
			return err
		}
		if _, err := runtime.ResolveSavePath(meetingScreenshotOutputPath(runtime)); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithParam("--output").WithCause(err)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST(vcMeetingScreenshotAPIPath).
			Body(meetingScreenshotRequest{MeetingID: strings.TrimSpace(runtime.Str("meeting-id"))}).
			Set("output", meetingScreenshotOutputPath(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		meetingID := strings.TrimSpace(runtime.Str("meeting-id"))
		outputPath := meetingScreenshotOutputPath(runtime)
		if _, err := runtime.ResolveSavePath(outputPath); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithParam("--output").WithCause(err)
		}
		if !runtime.Bool("overwrite") {
			if _, statErr := runtime.FileIO().Stat(outputPath); statErr == nil {
				return errs.NewValidationError(errs.SubtypeFailedPrecondition, "output file already exists: %s (use --overwrite to replace)", outputPath).WithParam("--output")
			}
		}
		stopSpinner := runtime.StartSpinner("Capturing meeting screenshot")
		defer stopSpinner()
		resp, err := runtime.DoAPIWithContext(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodPost,
			ApiPath:    vcMeetingScreenshotAPIPath,
			Body:       meetingScreenshotRequest{MeetingID: meetingID},
		}, larkcore.WithFileDownload())
		if err != nil {
			return err
		}

		rawContentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
		contentType, _, contentTypeErr := mime.ParseMediaType(rawContentType)
		if contentTypeErr != nil {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid screenshot content type %q", rawContentType).WithCause(contentTypeErr)
		}
		isJSON := contentType == "application/json" || contentType == "text/json"
		if contentType != "image/jpeg" && !isJSON {
			if resp.StatusCode >= http.StatusInternalServerError {
				return errs.NewNetworkError(errs.SubtypeNetworkServer, "screenshot request failed: HTTP %d", resp.StatusCode).
					WithCode(resp.StatusCode).
					WithRetryable()
			}
			if resp.StatusCode >= http.StatusBadRequest {
				return errs.NewAPIError(errs.SubtypeUnknown, "screenshot request failed: HTTP %d", resp.StatusCode).
					WithCode(resp.StatusCode)
			}
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "unexpected screenshot content type %q, want image/jpeg", rawContentType)
		}
		image := resp.RawBody
		if len(image) > meetingScreenshotMaxBytes {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "screenshot exceeds %d bytes", meetingScreenshotMaxBytes)
		}
		if isJSON {
			_, classifyErr := runtime.ClassifyAPIResponse(resp)
			if classifyErr != nil {
				return classifyErr
			}
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "unexpected screenshot JSON response")
		}
		if resp.StatusCode >= http.StatusBadRequest {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "unexpected screenshot HTTP status %d", resp.StatusCode).
				WithCode(resp.StatusCode)
		}
		if !validMeetingScreenshotJPEG(image) {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid screenshot JPEG response")
		}
		result, err := runtime.FileIO().Save(outputPath, fileio.SaveOptions{
			ContentType:   contentType,
			ContentLength: int64(len(image)),
		}, bytes.NewReader(image))
		if err != nil {
			return common.WrapSaveErrorTyped(err)
		}
		savedPath, err := runtime.ResolveSavePath(outputPath)
		if err != nil {
			return errs.NewInternalError(errs.SubtypeUnknown, "resolve saved screenshot path: %v", err).WithCause(err)
		}
		stopSpinner()
		digest := sha256.Sum256(image)
		runtime.Out(map[string]interface{}{
			"meeting_id":   meetingID,
			"path":         savedPath,
			"size_bytes":   result.Size(),
			"content_type": contentType,
			"sha256":       fmt.Sprintf("%x", digest),
		}, nil)
		return nil
	},
}

func meetingScreenshotOutputPath(runtime *common.RuntimeContext) string {
	if outputPath := strings.TrimSpace(runtime.Str("output")); outputPath != "" {
		return outputPath
	}
	meetingID := strings.TrimSpace(runtime.Str("meeting-id"))
	stamp := meetingScreenshotNow().UTC().Format("20060102T150405.000Z")
	return filepath.Join("meeting-screenshots", fmt.Sprintf("%s-%s.jpg", meetingID, stamp))
}

func validMeetingScreenshotJPEG(image []byte) bool {
	if len(image) < 4 || len(image) > meetingScreenshotMaxBytes ||
		image[0] != 0xff || image[1] != 0xd8 || image[len(image)-2] != 0xff || image[len(image)-1] != 0xd9 {
		return false
	}
	_, err := jpeg.Decode(bytes.NewReader(image))
	return err == nil
}
