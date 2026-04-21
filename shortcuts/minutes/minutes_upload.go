// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	minutesUploadSupportedFormatsTip = "Supported audio formats: wav, mp3, m4a, aac, ogg, wma, amr; supported video formats: avi, wmv, mov, mp4, m4v, mpeg, ogg, flv."
	minutesUploadLimitsTip           = "The original uploaded media must be no larger than 6GB and no longer than 6 hours."
)

// MinutesUpload uploads a media file token to generate a minute.
var MinutesUpload = common.Shortcut{
	Service:     "minutes",
	Command:     "+upload",
	Description: "Upload a media file token to generate a minute",
	Risk:        "write",
	Scopes:      []string{"minutes:minutes.upload:write"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "file-token", Desc: "file_token of a supported audio/video file already uploaded to Drive", Required: true},
	},
	Tips: []string{
		"This shortcut only accepts --file-token. Upload the local media file to Drive first with `lark-cli drive +upload`.",
		minutesUploadSupportedFormatsTip,
		minutesUploadLimitsTip,
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := runtime.Str("file-token")
		if fileToken == "" {
			return output.ErrValidation("--file-token is required")
		}
		if err := validate.ResourceName(fileToken, "--file-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST("/open-apis/minutes/v1/minutes/upload").
			Body(map[string]interface{}{"file_token": runtime.Str("file-token")})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := runtime.Str("file-token")

		body := map[string]interface{}{
			"file_token": fileToken,
		}

		data, err := runtime.CallAPI("POST", "/open-apis/minutes/v1/minutes/upload", nil, body)
		if err != nil {
			return err
		}

		minuteURL := common.GetString(data, "minute_url")

		outData := map[string]interface{}{
			"minute_url": minuteURL,
		}

		runtime.OutFormat(outData, nil, nil)
		return nil
	},
}
