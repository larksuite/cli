// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var commentTargetTypes = map[string]bool{
	"cycle": true, "progress": true, "objective": true, "key_result": true,
}

func validateCommentID(param, value string) error {
	if value == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s is required", param).WithParam(param)
	}
	if id, err := strconv.ParseInt(value, 10, 64); err != nil || id <= 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s must be a positive int64", param).WithParam(param)
	}
	return nil
}

func validateCommentStyle(runtime *common.RuntimeContext) error {
	if style := runtime.Str("style"); style != "simple" && style != "richtext" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--style must be one of: simple | richtext").WithParam("--style")
	}
	return nil
}

func validateCommentUserIDType(runtime *common.RuntimeContext) error {
	switch runtime.Str("user-id-type") {
	case "open_id", "union_id", "user_id", "user_key":
		return nil
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--user-id-type must be one of: open_id | union_id | user_id | user_key").WithParam("--user-id-type")
	}
}

func validateCommentTarget(runtime *common.RuntimeContext) error {
	if !commentTargetTypes[runtime.Str("target-type")] {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--target-type must be one of: cycle | progress | objective | key_result").WithParam("--target-type")
	}
	return validateCommentID("--target-id", runtime.Str("target-id"))
}

func commentQuery(runtime *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{"user_id_type": runtime.Str("user-id-type")}
}

func parseComment(data interface{}) (*Comment, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid comment response: %s", err).WithCause(err)
	}
	var result Comment
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid comment response: %s", err).WithCause(err)
	}
	return &result, nil
}

func parseCommentContent(runtime *common.RuntimeContext) (*ContentBlock, error) {
	value := runtime.Str("content")
	if value == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content is required").WithParam("--content")
	}
	if err := common.RejectDangerousCharsTyped("--content", value); err != nil {
		return nil, err
	}
	if runtime.Str("style") == "simple" {
		var simple SemiPlainContent
		if err := json.Unmarshal([]byte(value), &simple); err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content must be valid semi-plain JSON: %s", err).WithParam("--content").WithCause(err)
		}
		if strings.TrimSpace(simple.Text) == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content text is required and cannot be empty").WithParam("--content")
		}
		if len(simple.Docs) > 0 || len(simple.Images) > 0 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content docs and images are not supported in simple style input; use richtext style").WithParam("--content")
		}
		return simple.ToContentBlock(), nil
	}
	var content ContentBlock
	if err := json.Unmarshal([]byte(value), &content); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content must be valid ContentBlock JSON: %s", err).WithParam("--content").WithCause(err)
	}
	return &content, nil
}

func commentResponse(data map[string]interface{}) (*Comment, error) {
	value, ok := data["comment"]
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "comment response is missing comment")
	}
	return parseComment(value)
}

func commentListResponse(data map[string]interface{}) ([]Comment, bool, string, error) {
	raw, _ := data["items"].([]interface{})
	items := make([]Comment, 0, len(raw))
	for _, item := range raw {
		comment, err := parseComment(item)
		if err != nil {
			return nil, false, "", err
		}
		items = append(items, *comment)
	}
	hasMore, token := common.PaginationMeta(data)
	return items, hasMore, token, nil
}

func commentOutput(style string, comments []Comment) []*RespComment {
	result := make([]*RespComment, 0, len(comments))
	for i := range comments {
		result = append(result, comments[i].ToResp(style))
	}
	return result
}

func commentThreadOutput(style string, comments []Comment) [][]*RespComment {
	return responseCommentThreads(groupCommentThreads(comments), style)
}

func commentIDFlags() []common.Flag {
	return []common.Flag{
		{Name: "comment-id", Desc: "comment ID (int64)", Required: true},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type: open_id | union_id | user_id | user_key"}}
}

// OKRListComments lists one page of comments attached to an OKR entity.
var OKRListComments = common.Shortcut{
	Service:     "okr",
	Command:     "+comment-list",
	Description: "List comments attached to an OKR entity",
	Risk:        "read",
	Scopes:      []string{"okr:okr.comment.readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "target-id", Desc: "comment target ID (int64)", Required: true},
		{Name: "target-type", Desc: "comment target type: cycle | progress | objective | key_result", Required: true, Enum: []string{"cycle", "progress", "objective", "key_result"}},
		{Name: "page-size", Type: "int", Default: "100", Desc: "page size, range 1-100"},
		{Name: "page-token", Desc: "pagination token from previous response"},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type: open_id | union_id | user_id | user_key"},
		{Name: "style", Default: "simple", Desc: "output style: simple (semi-plain content) | richtext (ContentBlock)", Enum: []string{"simple", "richtext"}}},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateCommentTarget(runtime); err != nil {
			return err
		}
		if err := validateCommentUserIDType(runtime); err != nil {
			return err
		}
		if err := validateCommentStyle(runtime); err != nil {
			return err
		}
		_, err := common.ValidatePageSizeTyped(runtime, "page-size", 100, 1, 100)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params := commentQuery(runtime)
		params["target_type"] = runtime.Str("target-type")
		params["target_id"] = runtime.Str("target-id")
		params["page_size"] = runtime.Int("page-size")
		if token := runtime.Str("page-token"); token != "" {
			params["page_token"] = token
		}
		return common.NewDryRunAPI().GET("/open-apis/okr/v2/comments").Params(params).Desc("List OKR comments")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		params := commentQuery(runtime)
		params["target_type"] = runtime.Str("target-type")
		params["target_id"] = runtime.Str("target-id")
		params["page_size"] = runtime.Int("page-size")
		if token := runtime.Str("page-token"); token != "" {
			params["page_token"] = token
		}
		data, err := runtime.CallAPITyped("GET", "/open-apis/okr/v2/comments", params, nil)
		if err != nil {
			return err
		}
		comments, hasMore, token, err := commentListResponse(data)
		if err != nil {
			return err
		}
		runtime.OutFormat(map[string]interface{}{"comments": commentThreadOutput(runtime.Str("style"), comments), "has_more": hasMore, "page_token": token, "style": runtime.Str("style")}, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Found %d comment(s) in %d thread(s)\n", len(comments), len(groupCommentThreads(comments)))
		})
		return nil
	},
}

// OKRGetComment gets a comment by ID.
var OKRGetComment = common.Shortcut{
	Service:     "okr",
	Command:     "+comment-get",
	Description: "Get an OKR comment by ID",
	Risk:        "read",
	Scopes:      []string{"okr:okr.comment.readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       append(commentIDFlags(), common.Flag{Name: "style", Default: "simple", Desc: "output style: simple | richtext", Enum: []string{"simple", "richtext"}}),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateCommentID("--comment-id", runtime.Str("comment-id")); err != nil {
			return err
		}
		if err := validateCommentUserIDType(runtime); err != nil {
			return err
		}
		return validateCommentStyle(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().GET("/open-apis/okr/v2/comments/:comment_id").Params(commentQuery(runtime)).Set("comment_id", runtime.Str("comment-id")).Desc("Get OKR comment")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data, err := runtime.CallAPITyped("GET", fmt.Sprintf("/open-apis/okr/v2/comments/%s", runtime.Str("comment-id")), commentQuery(runtime), nil)
		if err != nil {
			return err
		}
		comment, err := commentResponse(data)
		if err != nil {
			return err
		}
		runtime.OutFormat(map[string]interface{}{"comment": comment.ToResp(runtime.Str("style")), "style": runtime.Str("style")}, nil, func(w io.Writer) { fmt.Fprintf(w, "Comment [%s]\n", comment.ID) })
		return nil
	},
}

func validateCreateSelection(runtime *common.RuntimeContext) error {
	targetType := runtime.Str("target-type")
	selected := runtime.Str("selected-text")
	ref := runtime.Str("ref-comment-id")
	all := runtime.Bool("select-all")
	if all && selected != "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--select-all and --selected-text are mutually exclusive").WithParam("--select-all")
	}
	if ref != "" {
		if err := validateCommentID("--ref-comment-id", ref); err != nil {
			return err
		}
		if selected != "" || all {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--ref-comment-id is mutually exclusive with --selected-text and --select-all").WithParam("--ref-comment-id")
		}
	}
	if selected != "" {
		if err := common.RejectDangerousCharsTyped("--selected-text", selected); err != nil {
			return err
		}
	}
	if targetType == "objective" || targetType == "key_result" {
		if (selected == "" && !all) == (ref == "") {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "objective/key_result comments require exactly one of --selected-text, --select-all, or --ref-comment-id").WithParam("--selected-text")
		}
	} else if selected != "" || all {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--selected-text and --select-all are only supported for objective/key_result comments").WithParam("--selected-text")
	}
	return nil
}

func createCommentBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	body := map[string]interface{}{"target": CommentTarget{TargetType: runtime.Str("target-type"), TargetID: runtime.Str("target-id")}}
	content, err := parseCommentContent(runtime)
	if err != nil {
		return nil, err
	}
	body["content"] = content
	if selected := runtime.Str("selected-text"); selected != "" {
		body["selected_text"] = selected
	}
	if runtime.Bool("select-all") {
		plain := content.ToSemiPlain()
		if plain != nil {
			body["selected_text"] = strings.Repeat("*", len([]rune(plain.Text)))
		}
	}
	if ref := runtime.Str("ref-comment-id"); ref != "" {
		body["ref_comment_id"] = ref
	}
	return body, nil
}

// OKRCreateComment creates or replies to an OKR comment.
var OKRCreateComment = common.Shortcut{
	Service:     "okr",
	Command:     "+comment-create",
	Description: "Create or reply to an OKR comment",
	Risk:        "write", Scopes: []string{"okr:okr.comment.writeonly"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "target-id", Desc: "comment target ID (int64)", Required: true},
		{Name: "target-type", Desc: "comment target type: cycle | progress | objective | key_result", Required: true, Enum: []string{"cycle", "progress", "objective", "key_result"}},
		{Name: "content", Desc: "comment content: semi-plain JSON or ContentBlock JSON according to --style", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "selected-text", Desc: "selected text for a new objective/key_result selection comment"},
		{Name: "select-all", Type: "bool", Desc: "use wildcard selection to trigger full-text selection"},
		{Name: "ref-comment-id", Desc: "comment ID to reply to or attach to an existing selection"},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type: open_id | union_id | user_id | user_key"},
		{Name: "style", Default: "simple", Desc: "input/output style: simple | richtext", Enum: []string{"simple", "richtext"}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateCommentTarget(runtime); err != nil {
			return err
		}
		if err := validateCommentStyle(runtime); err != nil {
			return err
		}
		if err := validateCommentUserIDType(runtime); err != nil {
			return err
		}
		if err := validateCreateSelection(runtime); err != nil {
			return err
		}
		_, err := parseCommentContent(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := createCommentBody(runtime)
		return common.NewDryRunAPI().POST("/open-apis/okr/v2/comments").Params(commentQuery(runtime)).Body(body).Desc("Create OKR comment")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := createCommentBody(runtime)
		if err != nil {
			return err
		}
		data, err := runtime.CallAPITyped("POST", "/open-apis/okr/v2/comments", commentQuery(runtime), body)
		if err != nil {
			return err
		}
		runtime.OutFormat(map[string]interface{}{"comment_id": data["comment_id"], "selection_id": data["selection_id"]}, nil, func(w io.Writer) { fmt.Fprintf(w, "Created comment %v\n", data["comment_id"]) })
		return nil
	},
}

// OKRPatchComment updates only a comment's content.
var OKRPatchComment = common.Shortcut{
	Service:     "okr",
	Command:     "+comment-patch",
	Description: "Update an OKR comment",
	Risk:        "write",
	Scopes:      []string{"okr:okr.comment.writeonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "comment-id", Desc: "comment ID (int64)", Required: true},
		{Name: "content", Desc: "new comment content: semi-plain JSON or ContentBlock JSON according to --style", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type: open_id | union_id | user_id | user_key"},
		{Name: "style", Default: "simple", Desc: "input/output style: simple | richtext", Enum: []string{"simple", "richtext"}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateCommentID("--comment-id", runtime.Str("comment-id")); err != nil {
			return err
		}
		if err := validateCommentUserIDType(runtime); err != nil {
			return err
		}
		if err := validateCommentStyle(runtime); err != nil {
			return err
		}
		_, err := parseCommentContent(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		content, _ := parseCommentContent(runtime)
		return common.NewDryRunAPI().PATCH("/open-apis/okr/v2/comments/:comment_id").Params(commentQuery(runtime)).Body(map[string]interface{}{"content": content}).Set("comment_id", runtime.Str("comment-id")).Desc("Update OKR comment")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		content, err := parseCommentContent(runtime)
		if err != nil {
			return err
		}
		data, err := runtime.CallAPITyped("PATCH", fmt.Sprintf("/open-apis/okr/v2/comments/%s", runtime.Str("comment-id")), commentQuery(runtime), map[string]interface{}{"content": content})
		if err != nil {
			return err
		}
		comment, err := commentResponse(data)
		if err != nil {
			return err
		}
		runtime.OutFormat(map[string]interface{}{"comment": comment.ToResp(runtime.Str("style")), "style": runtime.Str("style")}, nil, func(w io.Writer) { fmt.Fprintf(w, "Updated comment [%s]\n", comment.ID) })
		return nil
	},
}

func commentAction(command, suffix string) common.Shortcut {
	flags := append(commentIDFlags(), common.Flag{Name: "style", Default: "simple", Desc: "output style: simple | richtext", Enum: []string{"simple", "richtext"}})
	description := "Solve an OKR comment"
	if suffix == "reopen" {
		description = "Reopen an OKR comment"
	}
	return common.Shortcut{
		Service:     "okr",
		Command:     command,
		Description: description,
		Risk:        "write",
		Scopes:      []string{"okr:okr.comment.writeonly"},
		AuthTypes:   []string{"user"},
		HasFormat:   true,
		Flags:       flags,
		Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
			if err := validateCommentStyle(runtime); err != nil {
				return err
			}
			if err := validateCommentUserIDType(runtime); err != nil {
				return err
			}
			return validateCommentID("--comment-id", runtime.Str("comment-id"))
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			return common.NewDryRunAPI().POST("/open-apis/okr/v2/comments/:comment_id/"+suffix).Params(commentQuery(runtime)).Set("comment_id", runtime.Str("comment-id")).Desc(command)
		},
		Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
			data, err := runtime.CallAPITyped("POST", fmt.Sprintf("/open-apis/okr/v2/comments/%s/%s", runtime.Str("comment-id"), suffix), commentQuery(runtime), nil)
			if err != nil {
				return err
			}
			raw, _ := data["affected_comments"].([]interface{})
			comments := make([]Comment, 0, len(raw))
			for _, item := range raw {
				comment, e := parseComment(item)
				if e != nil {
					return e
				}
				comments = append(comments, *comment)
			}
			runtime.OutFormat(map[string]interface{}{"affected_comments": commentOutput(runtime.Str("style"), comments), "style": runtime.Str("style")}, nil, func(w io.Writer) { fmt.Fprintf(w, "Updated %d comment(s)\n", len(comments)) })
			return nil
		}}
}

// OKRSolveComment solves a comment or its selection thread.
var OKRSolveComment = commentAction("+comment-solve", "solve")

// OKRReopenComment reopens a comment or its selection thread.
var OKRReopenComment = commentAction("+comment-reopen", "reopen")

// OKRDeleteComment permanently deletes a comment.
var OKRDeleteComment = common.Shortcut{
	Service:     "okr",
	Command:     "+comment-delete",
	Description: "Delete an OKR comment permanently",
	Risk:        "high-risk-write",
	Scopes:      []string{"okr:okr.comment.delete"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "comment-id", Desc: "comment ID (int64)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateCommentID("--comment-id", runtime.Str("comment-id"))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().DELETE("/open-apis/okr/v2/comments/:comment_id").Set("comment_id", runtime.Str("comment-id")).Desc("Delete OKR comment")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := runtime.CallAPITyped("DELETE", fmt.Sprintf("/open-apis/okr/v2/comments/%s", runtime.Str("comment-id")), nil, nil)
		if err != nil {
			return err
		}
		runtime.OutFormat(map[string]interface{}{"deleted": true, "comment_id": runtime.Str("comment-id")}, nil, func(w io.Writer) { fmt.Fprintf(w, "Deleted comment %s\n", runtime.Str("comment-id")) })
		return nil
	}}
