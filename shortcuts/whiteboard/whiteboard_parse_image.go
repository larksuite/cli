// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

var wbParseImageScopes = []string{"board:whiteboard:node:create"}
var wbParseImageAuthTypes = []string{"user", "bot"}
var wbParseImageFlags = []common.Flag{
	{Name: "whiteboard-token", Desc: "whiteboard token of the target whiteboard. You need edit permission on the whiteboard.", Required: true},
	{Name: "image", Desc: "local image path to parse into whiteboard content. Supports PNG, JPG, JPEG, GIF, and WEBP. Shorthand: -i.", Required: true},
	{Name: "overwrite", Desc: "overwrite existing whiteboard content instead of appending. Default is false.", Required: false, Type: "bool"},
	{Name: "mode", Desc: "Canvas Agent mode. Empty defaults to flash on the server.", Required: false, Enum: []string{parseImageModeMini, parseImageModeFlash, parseImageModeAgentic, parseImageModeAgenticMax}},
	{Name: "idempotent-token", Desc: "idempotent token for the submit request. Default is a generated UUID. Minimum length is 10 when provided.", Required: false},
}

func wbParseImageValidate(_ context.Context, runtime *common.RuntimeContext) error {
	if err := common.RejectDangerousCharsTyped("--whiteboard-token", runtime.Str("whiteboard-token")); err != nil {
		return err
	}
	if err := validateParseImageFilePath(runtime.Str("image")); err != nil {
		return err
	}
	if runtime.Changed("mode") && strings.TrimSpace(runtime.Str("mode")) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--mode must be one of mini, flash, agentic, agentic_max").
			WithParam("--mode")
	}
	if _, err := normalizeParseImageMode(runtime.Str("mode")); err != nil {
		return err
	}
	_, err := normalizeParseImageIdempotentToken(runtime.Str("idempotent-token"))
	return err
}

func wbParseImageDryRun(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	clientToken, err := normalizeParseImageIdempotentToken(runtime.Str("idempotent-token"))
	if err != nil {
		return common.NewDryRunAPI().Desc("invalid idempotent token: " + err.Error())
	}
	mode, err := normalizeParseImageMode(runtime.Str("mode"))
	if err != nil {
		return common.NewDryRunAPI().Desc("invalid mode: " + err.Error())
	}
	body := map[string]interface{}{
		"image_file":   "@" + runtime.Str("image"),
		"overwrite":    runtime.Bool("overwrite"),
		"client_token": clientToken,
	}
	if mode != "" {
		body["mode"] = mode
	}
	return common.NewDryRunAPI().
		POST(wbParseImageDryRunURL(runtime.Str("whiteboard-token"))).
		Body(body).
		Desc("submit one image for automatic parse and write into the whiteboard.")
}

func wbParseImageExecute(_ context.Context, runtime *common.RuntimeContext) error {
	imagePath := runtime.Str("image")
	clientToken, err := normalizeParseImageIdempotentToken(runtime.Str("idempotent-token"))
	if err != nil {
		return err
	}
	mode, err := normalizeParseImageMode(runtime.Str("mode"))
	if err != nil {
		return err
	}
	info, err := runtime.FileIO().Stat(imagePath)
	if err != nil {
		return parseImageInputError(err)
	}
	formFields := map[string]any{
		"overwrite":    runtime.Bool("overwrite"),
		"client_token": clientToken,
	}
	if mode != "" {
		formFields["mode"] = mode
	}
	fd, err := cmdutil.BuildFormdata(
		runtime.FileIO(),
		"image_file",
		imagePath,
		false,
		nil,
		formFields,
	)
	if err != nil {
		return parseImageInputError(err)
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Submitting image for ParseImage: %s (%s)\n", filepath.Base(imagePath), common.FormatSize(info.Size()))

	resp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: "POST",
		ApiPath:    wbParseImageURL(runtime.Str("whiteboard-token")),
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		return wrapWbNetworkErr(err, "submit parse image failed: %v", err)
	}
	data, err := runtime.ClassifyAPIResponse(resp)
	if err != nil {
		return err
	}
	taskID := common.GetString(data, "task_id")
	status := common.GetString(data, "status")
	if taskID == "" || status == "" {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "parse image submit response missing task_id or status")
	}
	out := map[string]interface{}{
		"task_id":      taskID,
		"status":       status,
		"next_command": parseImageResultCommand(runtime.Str("whiteboard-token"), taskID, runtime.As()),
	}
	runtime.OutFormat(out, nil, func(w io.Writer) {
		fmt.Fprintf(w, "ParseImage submitted: task_id=%s status=%s\n", taskID, status)
		fmt.Fprintf(w, "Next: %s", out["next_command"])
	})
	return nil
}

func parseImageResultCommand(whiteboardToken, taskID string, identity core.Identity) string {
	return fmt.Sprintf("lark-cli whiteboard +parse-image-result --whiteboard-token %s --task-id %s --wait --as %s --format json", whiteboardToken, taskID, string(identity))
}

func installParseImageShorthand(cmd *cobra.Command) {
	cmd.Flags().StringP("image-short", "i", "", "shorthand for --image")
	_ = cmd.Flags().MarkHidden("image-short")
	previous := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if previous != nil {
			if err := previous(c, args); err != nil {
				return err
			}
		}
		if c.Flags().Changed("image-short") && !c.Flags().Changed("image") {
			value, _ := c.Flags().GetString("image-short")
			return c.Flags().Set("image", value)
		}
		return nil
	}
}

// WhiteboardParseImage registers the `whiteboard +parse-image` shortcut.
var WhiteboardParseImage = common.Shortcut{
	Service:     "whiteboard",
	Command:     "+parse-image",
	Description: "Submit one image for automatic parsing and writing into an existing whiteboard.",
	Risk:        "write",
	Scopes:      wbParseImageScopes,
	AuthTypes:   wbParseImageAuthTypes,
	Flags:       wbParseImageFlags,
	Validate:    wbParseImageValidate,
	DryRun:      wbParseImageDryRun,
	Execute:     wbParseImageExecute,
	PostMount:   installParseImageShorthand,
}
