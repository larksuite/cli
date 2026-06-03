// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var DriveInspect = common.Shortcut{
	Service:           "drive",
	Command:           "+inspect",
	Description:       "Inspect a Lark document URL to get its type, title, and canonical token (with wiki unwrapping)",
	Risk:              "read",
	Scopes:            []string{"drive:drive.metadata:readonly"},
	ConditionalScopes: []string{"wiki:node:retrieve"},
	AuthTypes:         []string{"user", "bot"},
	HasFormat:         true,
	Flags: []common.Flag{
		{
			Name:     "url",
			Desc:     "Lark/Feishu document URL (docx, doc, sheet, bitable, wiki, file, folder, mindnote, slides)",
			Required: true,
		},
		{
			Name: "type",
			Desc: "document type (required when --url is a bare token; auto-detected for URLs)",
			Enum: []string{"doc", "docx", "sheet", "bitable", "wiki", "file", "folder", "mindnote", "slides"},
		},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		raw := strings.TrimSpace(runtime.Str("url"))
		if raw == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--url cannot be empty").WithParam("--url")
		}

		_, ok := common.ParseResourceURL(raw)
		if !ok {
			// Not a recognized URL pattern.
			if strings.Contains(raw, "://") {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported --url %q: use a recognized Lark document URL or a bare token with --type", raw).WithParam("--url")
			}
			// Bare token: --type is required.
			if strings.TrimSpace(runtime.Str("type")) == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--type is required when --url is a bare token (allowed: doc, docx, sheet, bitable, wiki, file, folder, mindnote, slides)").WithParam("--type")
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		raw := strings.TrimSpace(runtime.Str("url"))
		ref, ok := common.ParseResourceURL(raw)
		if !ok {
			ref = common.ResourceRef{
				Type:  strings.TrimSpace(runtime.Str("type")),
				Token: raw,
			}
		}

		dry := common.NewDryRunAPI()

		if ref.Type == "wiki" {
			dry.Desc("2-step: inspect wiki node, then batch query metadata")
			dry.GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Inspect wiki node to get underlying document").
				Params(map[string]interface{}{"token": ref.Token})
			dry.POST("/open-apis/drive/v1/metas/batch_query").
				Desc("[2] Batch query document metadata (title)").
				Body(map[string]interface{}{
					"request_docs": []map[string]interface{}{
						{"doc_token": "<obj_token from step 1>", "doc_type": "<obj_type from step 1>"},
					},
				})
			return dry
		}

		dry.Desc("1-step: batch query document metadata")
		dry.POST("/open-apis/drive/v1/metas/batch_query").
			Body(map[string]interface{}{
				"request_docs": []map[string]interface{}{
					{"doc_token": ref.Token, "doc_type": ref.Type},
				},
			})
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		raw := strings.TrimSpace(runtime.Str("url"))

		// Step 1: Parse URL to extract {type, token}.
		ref, ok := common.ParseResourceURL(raw)
		if !ok {
			// Bare token: use --type.
			ref = common.ResourceRef{
				Type:  strings.TrimSpace(runtime.Str("type")),
				Token: raw,
			}
		}

		inputURL := raw
		docType := ref.Type
		docToken := ref.Token

		var wikiNode map[string]interface{}

		// Step 2: If type is "wiki", unwrap via get_node API.
		if docType == "wiki" {
			fmt.Fprintf(runtime.IO().ErrOut, "Inspecting wiki node: %s\n", common.MaskToken(docToken))
			data, err := runtime.CallAPITyped(
				"GET",
				"/open-apis/wiki/v2/spaces/get_node",
				map[string]interface{}{"token": docToken},
				nil,
			)
			if err != nil {
				return err
			}

			node := common.GetMap(data, "node")
			objType := common.GetString(node, "obj_type")
			objToken := common.GetString(node, "obj_token")
			spaceID := common.GetString(node, "space_id")
			nodeToken := common.GetString(node, "node_token")

			if objType == "" || objToken == "" {
				return errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki get_node returned incomplete node data (obj_type=%q, obj_token=%q)", objType, objToken)
			}

			wikiNode = map[string]interface{}{
				"space_id":   spaceID,
				"node_token": nodeToken,
				"obj_token":  objToken,
				"obj_type":   objType,
			}

			docType = objType
			docToken = objToken

			fmt.Fprintf(runtime.IO().ErrOut, "Wiki unwrapped to %s: %s\n", docType, common.MaskToken(docToken))
		}

		// Step 3: Call batch_query to verify and get title.
		title, err := common.FetchDriveMetaTitle(runtime, docToken, docType)
		if err != nil {
			return err
		}

		// Step 4: Build the resolved URL.
		resolvedURL := common.BuildResourceURL(runtime.Config.Brand, docType, docToken)

		// Step 5: Build output.
		result := map[string]interface{}{
			"input_url": inputURL,
			"type":      docType,
			"title":     title,
			"token":     docToken,
			"url":       resolvedURL,
		}
		if wikiNode != nil {
			result["wiki_node"] = wikiNode
		}

		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Type:  %s\n", docType)
			if title != "" {
				fmt.Fprintf(w, "Title: %s\n", title)
			}
			fmt.Fprintf(w, "Token: %s\n", docToken)
			if resolvedURL != "" {
				fmt.Fprintf(w, "URL:   %s\n", resolvedURL)
			}
			if wikiNode != nil {
				fmt.Fprintf(w, "Wiki:  space_id=%s, node_token=%s\n", wikiNode["space_id"], wikiNode["node_token"])
			}
		})
		return nil
	},
}
