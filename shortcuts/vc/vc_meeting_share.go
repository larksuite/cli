// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	meetingShareAPIPath       = "/open-apis/vc/v1/bots/meeting_share"
	meetingShareStatusAPIPath = "/open-apis/vc/v1/bots/meeting_share_status"

	meetingShareActionStart         = "start"
	meetingShareActionStop          = "stop"
	meetingShareActionUpdateContext = "update_context"
	meetingShareActionCommentPanel  = "comment_panel"
)

var (
	meetingShareActions        = []string{meetingShareActionStart, meetingShareActionStop, meetingShareActionUpdateContext, meetingShareActionCommentPanel}
	meetingShareElementActions = []string{"open", "close"}
	meetingShareElementTypes   = []string{"image", "whiteboard"}
)

// VCMeetingShare controls an in-meeting document share and its comment panel.
var VCMeetingShare = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-share",
	Description: "Start or stop a Doc, Docx, or Sheet share, update Docx context or comment panel",
	Risk:        "write",
	Scopes:      []string{"vc:meeting.interaction:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-id", Required: true, Desc: "long numeric meeting ID (not the 9-digit meeting number)"},
		{Name: "action", Required: true, Desc: "share action: start, stop, update_context, or comment_panel (last two: Docx/Wiki-Docx only)", Enum: meetingShareActions},
		{Name: "share-id", Desc: "positive integer share ID returned by start; required for stop, update_context, and comment_panel"},
		{Name: "doc-url", Desc: "HTTPS Doc, Docx, or Sheet URL, including Wiki URLs resolving to those types; required for start"},
		{Name: "comment-panel-open", Type: "bool", Desc: "open or close the comment panel (Docx/Wiki-Docx only); optional for start, explicitly required for comment_panel"},
		{Name: "block-id", Desc: "Docx block ID for a position update"},
		{Name: "block-y-ratio", Type: "float64", Desc: "position within the block from 0 to 1"},
		{Name: "comment-focus-shown", Type: "bool", Desc: "show or hide comment focus; true requires --comment-id and --comment-anchor-id; false preserves the current comment with --comment-id"},
		{Name: "comment-id", Desc: "comment ID; requires explicit --comment-focus-shown"},
		{Name: "comment-anchor-id", Desc: "comment anchor block ID; requires --comment-focus-shown=true"},
		{Name: "element-action", Desc: "element preview action: open or close", Enum: meetingShareElementActions},
		{Name: "element-type", Desc: "element preview type: image or whiteboard", Enum: meetingShareElementTypes},
		{Name: "element-block-id", Desc: "block ID of the document element preview"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateMeetingEventsMeetingID(runtime.Str("meeting-id")); err != nil {
			return err
		}
		_, err := buildMeetingShareBody(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := buildMeetingShareBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().POST(meetingShareAPIPath).Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := buildMeetingShareBody(runtime)
		if err != nil {
			return err
		}
		data, err := runtime.CallAPITyped(http.MethodPost, meetingShareAPIPath, nil, body)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}
		action, _ := body["action"].(string)
		runtime.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintln(w, "Meeting document share operated.")
			fmt.Fprintf(w, "  Action:  %s\n", action)
			if shareID := common.GetString(data, "share_id"); shareID != "" {
				fmt.Fprintf(w, "  Share ID:  %s\n", sanitizeMeetingSharePrettyText(shareID))
			}
		})
		return nil
	},
}

// VCMeetingShareStatus gets the active document share snapshot for a meeting.
var VCMeetingShareStatus = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-share-status",
	Description: "Get the active in-meeting document share snapshot",
	Risk:        "read",
	Scopes:      []string{"vc:meeting.realtime:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-id", Required: true, Desc: "long numeric meeting ID (not the 9-digit meeting number)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateMeetingEventsMeetingID(runtime.Str("meeting-id"))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET(meetingShareStatusAPIPath).
			Params(meetingShareStatusParams(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data, err := runtime.CallAPITyped(http.MethodGet, meetingShareStatusAPIPath, meetingShareStatusParams(runtime), nil)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}
		runtime.OutFormat(data, nil, func(w io.Writer) {
			share := common.GetMap(data, "share")
			if len(share) == 0 {
				fmt.Fprintln(w, "No active document share.")
				return
			}
			fmt.Fprintln(w, "Active document share:")
			if shareID := common.GetString(share, "share_id"); shareID != "" {
				fmt.Fprintf(w, "  Share ID:  %s\n", sanitizeMeetingSharePrettyText(shareID))
			}
			if title := common.GetString(share, "title"); title != "" {
				fmt.Fprintf(w, "  Title:  %s\n", sanitizeMeetingSharePrettyText(title))
			}
			if token := common.GetString(share, "doc_token"); token != "" {
				fmt.Fprintf(w, "  Document token:  %s\n", sanitizeMeetingSharePrettyText(token))
			}
		})
		return nil
	},
}

func meetingShareStatusParams(runtime *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{"meeting_id": strings.TrimSpace(runtime.Str("meeting-id"))}
}

func buildMeetingShareBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	action := strings.ToLower(strings.TrimSpace(runtime.Str("action")))
	if err := validateMeetingShareAction(action); err != nil {
		return nil, err
	}
	if runtime.Changed("comment-panel-open") && action != meetingShareActionStart && action != meetingShareActionCommentPanel {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--comment-panel-open is only supported when --action start or comment_panel").WithParam("--comment-panel-open")
	}
	body := map[string]interface{}{
		"meeting_id": strings.TrimSpace(runtime.Str("meeting-id")),
		"action":     action,
		"share_type": "document",
	}

	switch action {
	case meetingShareActionStart:
		document, err := buildMeetingShareStartDocument(runtime)
		if err != nil {
			return nil, err
		}
		body["document"] = document
	case meetingShareActionStop:
		if err := rejectMeetingShareDocumentFlags(runtime, meetingShareActionStop); err != nil {
			return nil, err
		}
		shareID, err := meetingSharePositiveID("--share-id", runtime.Str("share-id"))
		if err != nil {
			return nil, err
		}
		body["share_id"] = shareID
	case meetingShareActionUpdateContext:
		shareID, err := meetingSharePositiveID("--share-id", runtime.Str("share-id"))
		if err != nil {
			return nil, err
		}
		document, err := buildMeetingShareContext(runtime)
		if err != nil {
			return nil, err
		}
		body["share_id"] = shareID
		body["document"] = document
	case meetingShareActionCommentPanel:
		if err := rejectMeetingShareDocumentFlags(runtime, action); err != nil {
			return nil, err
		}
		shareID, err := meetingSharePositiveID("--share-id", runtime.Str("share-id"))
		if err != nil {
			return nil, err
		}
		if !runtime.Changed("comment-panel-open") {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--action comment_panel requires explicit --comment-panel-open=true or false").WithParam("--comment-panel-open")
		}
		body["share_id"] = shareID
		body["document"] = map[string]interface{}{"comment_panel_open": runtime.Bool("comment-panel-open")}
	}
	return body, nil
}

func validateMeetingShareAction(action string) error {
	switch action {
	case meetingShareActionStart, meetingShareActionStop, meetingShareActionUpdateContext, meetingShareActionCommentPanel:
		return nil
	case "":
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--action is required").WithParam("--action")
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--action must be one of start, stop, update_context, comment_panel").WithParam("--action")
	}
}

func buildMeetingShareStartDocument(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	if runtime.Changed("share-id") {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--share-id is not supported when --action start").WithParam("--share-id")
	}
	if err := rejectMeetingShareContextFlags(runtime); err != nil {
		return nil, err
	}
	docURL := strings.TrimSpace(runtime.Str("doc-url"))
	parsedURL, err := url.Parse(docURL)
	if err != nil || parsedURL == nil || parsedURL.Scheme != "https" ||
		parsedURL.Hostname() == "" || parsedURL.User != nil || len(docURL) > 4096 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--doc-url must be a complete HTTPS URL without user information when --action start").WithParam("--doc-url")
	}
	document := map[string]interface{}{"doc_url": docURL}
	if runtime.Changed("comment-panel-open") {
		document["comment_panel_open"] = runtime.Bool("comment-panel-open")
	}
	return document, nil
}

func rejectMeetingShareDocumentFlags(runtime *common.RuntimeContext, action string) error {
	if runtime.Changed("doc-url") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--doc-url is not supported when --action %s", action).WithParam("--doc-url")
	}
	return rejectMeetingShareContextFlags(runtime)
}

func rejectMeetingShareContextFlags(runtime *common.RuntimeContext) error {
	for _, name := range []string{
		"block-id", "block-y-ratio", "comment-focus-shown", "comment-id", "comment-anchor-id",
		"element-action", "element-type", "element-block-id",
	} {
		if runtime.Changed(name) {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s is only supported when --action update_context", name).WithParam("--" + name)
		}
	}
	return nil
}

func buildMeetingShareContext(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	if runtime.Changed("doc-url") {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--doc-url is only supported when --action start").WithParam("--doc-url")
	}
	positionSet := runtime.Changed("block-id") || runtime.Changed("block-y-ratio")
	commentSet := runtime.Changed("comment-focus-shown") || runtime.Changed("comment-id") || runtime.Changed("comment-anchor-id")
	elementSet := runtime.Changed("element-action") || runtime.Changed("element-type") || runtime.Changed("element-block-id")
	count := 0
	for _, set := range []bool{positionSet, commentSet, elementSet} {
		if set {
			count++
		}
	}
	if count == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--action update_context requires one position, comment, or element-preview context").WithParam("--block-id")
	}
	if count > 1 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "position, comment, and element-preview context flags are mutually exclusive").
			WithParams(meetingShareContextConflictParams(runtime)...)
	}
	switch {
	case positionSet:
		return buildMeetingSharePosition(runtime)
	case commentSet:
		return buildMeetingShareCommentContext(runtime)
	default:
		return buildMeetingShareElementContext(runtime)
	}
}

func buildMeetingSharePosition(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	blockID := strings.TrimSpace(runtime.Str("block-id"))
	if blockID == "" || len(blockID) > 256 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--block-id must contain 1 to 256 characters").WithParam("--block-id")
	}
	document := map[string]interface{}{"block_id": blockID}
	if runtime.Changed("block-y-ratio") {
		ratio := runtime.Float64("block-y-ratio")
		if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || ratio > 1 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--block-y-ratio must be between 0 and 1").WithParam("--block-y-ratio")
		}
		document["block_y_ratio"] = ratio
	}
	return document, nil
}

func buildMeetingShareCommentContext(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	document := map[string]interface{}{}
	focusSet := runtime.Changed("comment-focus-shown")
	commentSet := runtime.Changed("comment-id")
	anchorSet := runtime.Changed("comment-anchor-id")
	shown := runtime.Bool("comment-focus-shown")
	if !focusSet {
		if anchorSet {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--comment-anchor-id requires --comment-focus-shown=true").WithParam("--comment-anchor-id")
		}
		if commentSet {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--comment-id requires explicit --comment-focus-shown=true or false").WithParam("--comment-focus-shown")
		}
	}
	if focusSet {
		if shown && !commentSet {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--comment-focus-shown=true requires --comment-id").WithParam("--comment-id")
		}
		if shown && !anchorSet {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--comment-focus-shown=true requires --comment-anchor-id").WithParam("--comment-anchor-id")
		}
		if !shown && anchorSet {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--comment-anchor-id requires --comment-focus-shown=true").WithParam("--comment-anchor-id")
		}
		document["comment_focus_shown"] = shown
	}
	commentID := strings.TrimSpace(runtime.Str("comment-id"))
	if runtime.Changed("comment-id") {
		if commentID == "" || len(commentID) > 256 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--comment-id must contain 1 to 256 characters").WithParam("--comment-id")
		}
		document["comment_id"] = commentID
	}
	if runtime.Changed("comment-anchor-id") {
		anchorID := strings.TrimSpace(runtime.Str("comment-anchor-id"))
		if commentID == "" || anchorID == "" || len(anchorID) > 256 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--comment-anchor-id requires --comment-id and must contain 1 to 256 characters").WithParam("--comment-anchor-id")
		}
		document["comment_anchor_id"] = anchorID
	}
	return document, nil
}

func buildMeetingShareElementContext(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	action := strings.ToLower(strings.TrimSpace(runtime.Str("element-action")))
	if action != "open" && action != "close" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--element-action must be open or close").WithParam("--element-action")
	}
	elementType := strings.ToLower(strings.TrimSpace(runtime.Str("element-type")))
	if elementType != "image" && elementType != "whiteboard" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--element-type must be image or whiteboard").WithParam("--element-type")
	}
	blockID := strings.TrimSpace(runtime.Str("element-block-id"))
	if blockID == "" || len(blockID) > 256 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--element-block-id must contain 1 to 256 characters").WithParam("--element-block-id")
	}
	return map[string]interface{}{
		"element_preview": map[string]interface{}{
			"action":       action,
			"element_type": elementType,
			"block_id":     blockID,
		},
	}, nil
}

func meetingShareContextConflictParams(runtime *common.RuntimeContext) []errs.InvalidParam {
	const reason = "mutually exclusive with another document context"
	names := []string{
		"block-id", "block-y-ratio", "comment-focus-shown", "comment-id", "comment-anchor-id",
		"element-action", "element-type", "element-block-id",
	}
	params := make([]errs.InvalidParam, 0, len(names))
	for _, name := range names {
		if runtime.Changed(name) {
			params = append(params, errs.InvalidParam{Name: "--" + name, Reason: reason})
		}
	}
	return params
}

func sanitizeMeetingSharePrettyText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || common.IsDangerousUnicode(r) {
			return -1
		}
		return r
	}, value)
}

func meetingSharePositiveID(param, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "%s must be a positive integer", param).WithParam(param)
	}
	return value, nil
}
