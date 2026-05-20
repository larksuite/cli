// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	secureLabelReadScope   = "drive:file.meta.sec_label.read_only"
	secureLabelUpdateScope = "docs:secure_label:write_only"
)

var secureLabelTypes = permApplyTypes

// DriveSecureLabelList lists secure labels available to the current user.
var DriveSecureLabelList = common.Shortcut{
	Service:     "drive",
	Command:     "+secure-label-list",
	Description: "List secure labels available to the current user",
	Risk:        "read",
	Scopes:      []string{secureLabelReadScope},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "page-size", Type: "int", Default: "10", Desc: "page size, 1-10"},
		{Name: "page-token", Desc: "pagination token from previous response"},
		{Name: "lang", Desc: "label language", Enum: []string{"zh", "en", "ja"}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		pageSize := runtime.Int("page-size")
		if pageSize < 1 || pageSize > 10 {
			return output.ErrValidation("--page-size must be between 1 and 10")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			Desc("List secure labels available to the current user").
			GET("/open-apis/drive/v2/my_secure_labels").
			Params(buildSecureLabelListParams(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data, err := runtime.CallAPI("GET",
			"/open-apis/drive/v2/my_secure_labels",
			buildSecureLabelListParams(runtime),
			nil,
		)
		if err != nil {
			return err
		}
		runtime.OutFormat(data, nil, nil)
		return nil
	},
}

// DriveSecureLabelUpdate updates the secure label on a Drive file/document.
var DriveSecureLabelUpdate = common.Shortcut{
	Service:     "drive",
	Command:     "+secure-label-update",
	Description: "Update the secure label on a Drive file or document",
	Risk:        "write",
	Scopes:      []string{secureLabelUpdateScope},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "token", Desc: "target file token or document URL (docx/sheets/base/file/wiki/doc/mindnote/slides)", Required: true},
		{Name: "type", Desc: "target type; auto-inferred from URL when omitted", Enum: secureLabelTypes},
		{Name: "label-id", Desc: "secure label ID to set", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, _, err := resolveSecureLabelTarget(runtime.Str("token"), runtime.Str("type"))
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, docType, err := resolveSecureLabelTarget(runtime.Str("token"), runtime.Str("type"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			Desc("Update Drive secure label").
			PATCH("/open-apis/drive/v2/files/:file_token/secure_label").
			Params(map[string]interface{}{"type": docType}).
			Body(map[string]interface{}{"id": runtime.Str("label-id")}).
			Set("file_token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, docType, err := resolveSecureLabelTarget(runtime.Str("token"), runtime.Str("type"))
		if err != nil {
			return err
		}
		body := map[string]interface{}{"id": runtime.Str("label-id")}
		data, err := runtime.CallAPI("PATCH",
			fmt.Sprintf("/open-apis/drive/v2/files/%s/secure_label", validate.EncodePathSegment(token)),
			map[string]interface{}{"type": docType},
			body,
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

func buildSecureLabelListParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{"page_size": runtime.Int("page-size")}
	if pageToken := runtime.Str("page-token"); pageToken != "" {
		params["page_token"] = pageToken
	}
	if lang := runtime.Str("lang"); lang != "" {
		params["lang"] = lang
	}
	return params
}

func resolveSecureLabelTarget(raw, explicitType string) (token, docType string, err error) {
	return resolvePermApplyTarget(raw, explicitType)
}
