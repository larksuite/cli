// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	driveInspectRateLimitRetries    = 2
	driveInspectRetryInitialBackoff = 200 * time.Millisecond
)

var driveInspectAfter = time.After

var DriveInspect = common.Shortcut{
	Service:           "drive",
	Command:           "+inspect",
	Description:       "Inspect a Lark document URL to get its type, title, URL, create time, and canonical token (with wiki unwrapping)",
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
		if _, err := driveInspectResolveRef(runtime); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ref, err := driveInspectResolveRef(runtime)
		if err != nil {
			return common.NewDryRunAPI()
		}

		dry := common.NewDryRunAPI()

		if ref.Type == "wiki" {
			dry.Desc("2-step: inspect wiki node, then batch query metadata")
			dry.GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Inspect wiki node to get underlying document").
				Params(map[string]interface{}{"token": ref.Token})
			dry.POST("/open-apis/drive/v1/metas/batch_query").
				Desc("[2] Batch query document metadata (title, URL, and create time)").
				Body(map[string]interface{}{
					"request_docs": []map[string]interface{}{
						{"doc_token": "<obj_token from step 1>", "doc_type": "<obj_type from step 1>"},
					},
					"with_url": true,
				})
			return dry
		}

		dry.Desc("1-step: batch query document metadata")
		dry.POST("/open-apis/drive/v1/metas/batch_query").
			Body(map[string]interface{}{
				"request_docs": []map[string]interface{}{
					{"doc_token": ref.Token, "doc_type": ref.Type},
				},
				"with_url": true,
			})
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		raw := strings.TrimSpace(runtime.Str("url"))
		ref, err := driveInspectResolveRef(runtime)
		if err != nil {
			return err
		}

		inputURL := raw
		docType := ref.Type
		docToken := ref.Token

		var wikiNode map[string]interface{}

		// Step 2: If type is "wiki", unwrap via get_node API.
		if docType == "wiki" {
			fmt.Fprintf(runtime.IO().ErrOut, "Inspecting wiki node: %s\n", common.MaskToken(docToken))
			data, err := driveInspectCallWithRetry(
				ctx,
				func() (map[string]interface{}, error) {
					return runtime.CallAPITyped(
						"GET",
						"/open-apis/wiki/v2/spaces/get_node",
						map[string]interface{}{"token": docToken},
						nil,
					)
				},
			)
			if err != nil {
				return driveInspectAnnotateWikiResolveError(err)
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

		// Step 3: Call batch_query to verify and get metadata used by output and citations.
		meta, err := driveInspectFetchMeta(ctx, runtime, docToken, docType)
		if err != nil {
			return driveInspectAnnotateError("query_meta", err)
		}

		// Step 4: Prefer the tenant-native URL returned by batch_query. Keep the
		// previous synthesized URL as a compatibility fallback for incomplete
		// metadata responses.
		resolvedURL := meta.URL
		if resolvedURL == "" {
			resolvedURL = common.BuildResourceURL(runtime.Config.Brand, docType, docToken)
		}

		// Step 5: Build output.
		result := map[string]interface{}{
			"input_url":   inputURL,
			"type":        docType,
			"title":       meta.Title,
			"token":       docToken,
			"url":         resolvedURL,
			"create_time": meta.CreateTime,
		}
		if wikiNode != nil {
			result["wiki_node"] = wikiNode
		}

		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Type:  %s\n", docType)
			if meta.Title != "" {
				fmt.Fprintf(w, "Title: %s\n", meta.Title)
			}
			fmt.Fprintf(w, "Token: %s\n", docToken)
			if resolvedURL != "" {
				fmt.Fprintf(w, "URL:   %s\n", resolvedURL)
			}
			if meta.CreateTime != "" {
				fmt.Fprintf(w, "Create time: %s\n", meta.CreateTime)
			}
			if wikiNode != nil {
				fmt.Fprintf(w, "Wiki:  space_id=%s, node_token=%s\n", wikiNode["space_id"], wikiNode["node_token"])
			}
		})
		return nil
	},
	Citation: &common.CitationDefinition{
		SourceTypes: []citation.SourceType{
			citation.SourceDoc,
			citation.SourceBase,
			citation.SourceSheet,
			citation.SourceMindnote,
			citation.SourceSlides,
			citation.SourceFile,
		},
		Build: driveInspectCitations,
	},
}

func driveInspectResolveRef(runtime *common.RuntimeContext) (common.ResourceRef, error) {
	raw := strings.TrimSpace(runtime.Str("url"))
	if raw == "" {
		return common.ResourceRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--url cannot be empty").WithParam("--url")
	}

	inputType := strings.ToLower(strings.TrimSpace(runtime.Str("type")))
	ref, ok := common.ParseResourceURL(raw)
	if ok {
		if inputType != "" && inputType != ref.Type {
			return common.ResourceRef{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--type %q conflicts with URL path type %q; remove --type or use a matching value",
				inputType,
				ref.Type,
			).WithParam("--type")
		}
		return ref, nil
	}

	if strings.Contains(raw, "://") {
		return common.ResourceRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported --url %q: use a recognized Lark document URL or a bare token with --type", raw).WithParam("--url")
	}
	if strings.ContainsAny(raw, "/?#") {
		return common.ResourceRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid bare token %q: remove path/query fragments and pass only the raw token with --type", raw).WithParam("--url")
	}
	if inputType == "" {
		return common.ResourceRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--type is required when --url is a bare token (allowed: doc, docx, sheet, bitable, wiki, file, folder, mindnote, slides)").WithParam("--type")
	}
	return common.ResourceRef{Type: inputType, Token: raw}, nil
}

type driveInspectMeta struct {
	Title      string
	URL        string
	CreateTime string
}

func driveInspectFetchMeta(ctx context.Context, runtime *common.RuntimeContext, token, docType string) (driveInspectMeta, error) {
	var meta driveInspectMeta
	_, err := driveInspectCallWithRetry(ctx, func() (map[string]interface{}, error) {
		data, callErr := runtime.CallAPITyped(
			"POST",
			"/open-apis/drive/v1/metas/batch_query",
			nil,
			map[string]interface{}{
				"request_docs": []map[string]interface{}{{
					"doc_token": token,
					"doc_type":  docType,
				}},
				"with_url": true,
			},
		)
		if callErr != nil {
			return nil, callErr
		}
		metas := common.GetSlice(data, "metas")
		if len(metas) == 0 {
			return data, nil
		}
		raw, _ := metas[0].(map[string]interface{})
		meta = driveInspectMeta{
			Title:      common.GetString(raw, "title"),
			URL:        strings.TrimSpace(common.GetString(raw, "url")),
			CreateTime: common.GetStringLoose(raw, "create_time"),
		}
		return data, nil
	})
	if err != nil {
		return driveInspectMeta{}, err
	}
	return meta, nil
}

func driveInspectCallWithRetry(ctx context.Context, call func() (map[string]interface{}, error)) (map[string]interface{}, error) {
	var lastErr error
	for attempt := 0; attempt <= driveInspectRateLimitRetries; attempt++ {
		data, err := call()
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !driveInspectShouldRetry(err) || attempt == driveInspectRateLimitRetries {
			return nil, err
		}
		backoff := driveInspectRetryInitialBackoff * time.Duration(1<<attempt)
		if waitErr := driveInspectWait(ctx, backoff); waitErr != nil {
			return nil, waitErr
		}
	}
	return nil, lastErr
}

func driveInspectShouldRetry(err error) bool {
	problem, ok := errs.ProblemOf(err)
	if !ok || problem == nil {
		return false
	}
	return problem.Subtype == errs.SubtypeRateLimit || problem.Code == 99991400 || problem.Retryable
}

func driveInspectWait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return errs.WrapInternal(ctx.Err())
	case <-driveInspectAfter(d):
		return nil
	}
}

const (
	driveInspectWikiPermissionDeniedCode = 131006
	driveInspectWikiPermissionDeniedHint = "The current user or app/bot identity lacks access to the target wiki space or node. This is resource access, not app scope authorization. Do not retry the same request, reauthorize, or switch identity as trial and error; ask the resource owner or wiki administrator to grant read access, or use an accessible resource."
)

func driveInspectAnnotateWikiResolveError(err error) error {
	err = driveInspectAnnotateError("resolve_wiki", err)
	problem, ok := errs.ProblemOf(err)
	if ok && problem != nil && problem.Code == driveInspectWikiPermissionDeniedCode {
		problem.Hint = driveInspectWikiPermissionDeniedHint
	}
	return err
}

func driveInspectAnnotateError(stage string, err error) error {
	problem, ok := errs.ProblemOf(err)
	if !ok || problem == nil {
		return err
	}
	label := map[string]string{
		"resolve_wiki": "resolve wiki node",
		"query_meta":   "query document metadata",
	}[stage]
	if label == "" {
		label = stage
	}
	problem.Message = fmt.Sprintf("%s failed: %s", label, problem.Message)
	if strings.TrimSpace(problem.Hint) == "" {
		switch stage {
		case "resolve_wiki":
			problem.Hint = "check that the wiki URL/token is valid and that the current identity can read the wiki node"
		case "query_meta":
			problem.Hint = "check that the resolved document still exists and that the current identity can read its metadata"
		}
	} else if !strings.Contains(problem.Hint, label) {
		problem.Hint = label + ": " + problem.Hint
	}
	return err
}
