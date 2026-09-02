// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"golang.org/x/sync/errgroup"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const commentDetailConcurrency = 8

func commentAPIWithContext(ctx context.Context, runtime *common.RuntimeContext, method, path string, params map[string]interface{}) (map[string]interface{}, error) {
	query := make(larkcore.QueryParams, len(params))
	for key, value := range params {
		query.Set(key, fmt.Sprint(value))
	}
	resp, err := runtime.DoAPIWithContext(ctx, &larkcore.ApiReq{
		HttpMethod:  method,
		ApiPath:     path,
		QueryParams: query,
	})
	if err != nil {
		return nil, err
	}
	return runtime.ClassifyAPIResponse(resp)
}

type commentProgressPage struct {
	Items *[]Progress `json:"items"`
}

func decodeCommentProgressPage(data map[string]interface{}) ([]Progress, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid progress response: marshal failed: %s", err).WithCause(err)
	}
	var page commentProgressPage
	if err := json.Unmarshal(payload, &page); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid progress response: unmarshal failed: %s", err).WithCause(err)
	}
	if page.Items == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid progress response: missing progress response items")
	}
	return *page.Items, nil
}

func fetchComments(ctx context.Context, runtime *common.RuntimeContext, target CommentTarget) ([]Comment, error) {
	params := map[string]interface{}{"target_type": target.TargetType, "target_id": target.TargetID, "page_size": 100}
	var comments []Comment
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := commentAPIWithContext(ctx, runtime, "GET", "/open-apis/okr/v2/comments", params)
		if err != nil {
			return nil, err
		}
		items, hasMore, token, err := commentListResponse(data)
		if err != nil {
			return nil, err
		}
		comments = append(comments, items...)
		if !hasMore || token == "" {
			return comments, nil
		}
		params["page_token"] = token
	}
}

func fetchCommentProgresses(ctx context.Context, runtime *common.RuntimeContext, collection, targetID string) ([]Progress, error) {
	path := fmt.Sprintf("/open-apis/okr/v2/%s/%s/progresses", collection, targetID)
	params := map[string]interface{}{"page_size": 100}
	var progresses []Progress
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := commentAPIWithContext(ctx, runtime, "GET", path, params)
		if err != nil {
			return nil, err
		}
		items, err := decodeCommentProgressPage(data)
		if err != nil {
			return nil, err
		}
		progresses = append(progresses, items...)
		hasMore, token := common.PaginationMeta(data)
		if !hasMore || token == "" {
			return progresses, nil
		}
		params["page_token"] = token
	}
}

func commentTargetSortLess(a, b Comment) bool {
	if a.CreateTime != b.CreateTime {
		return a.CreateTime < b.CreateTime
	}
	return a.ID < b.ID
}

func groupCommentThreads(comments []Comment) [][]Comment {
	groups := make(map[string][]Comment)
	keys := make([]string, 0)
	for _, comment := range comments {
		key := "comment:" + comment.ID
		if comment.Selection != nil && comment.Selection.ID != "" {
			key = "selection:" + comment.Selection.ID
		}
		if _, ok := groups[key]; !ok {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], comment)
	}
	for _, key := range keys {
		sort.SliceStable(groups[key], func(i, j int) bool { return commentTargetSortLess(groups[key][i], groups[key][j]) })
	}
	sort.SliceStable(keys, func(i, j int) bool { return commentTargetSortLess(groups[keys[i]][0], groups[keys[j]][0]) })
	result := make([][]Comment, 0, len(keys))
	for _, key := range keys {
		result = append(result, groups[key])
	}
	return result
}

func responseCommentThreads(threads [][]Comment, style string) [][]*RespComment {
	result := make([][]*RespComment, 0, len(threads))
	for _, thread := range threads {
		converted := make([]*RespComment, 0, len(thread))
		for i := range thread {
			converted = append(converted, thread[i].ToResp(style))
		}
		result = append(result, converted)
	}
	return result
}

func fetchCommentDetailTargets(ctx context.Context, runtime *common.RuntimeContext, cycleID string) ([]CommentTarget, error) {
	objectives, err := fetchObjectives(ctx, runtime, cycleID)
	if err != nil {
		return nil, err
	}
	type loadedObjective struct {
		objective  Objective
		keyResults []KeyResult
	}
	loaded := make([]loadedObjective, len(objectives))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(commentDetailConcurrency)
	for obj := range objectives {
		i := obj
		g.Go(func() error {
			krs, err := fetchKeyResults(gctx, runtime, objectives[i].ID)
			if err != nil {
				return err
			}
			loaded[i] = loadedObjective{objectives[i], krs}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	targets := []CommentTarget{{TargetType: "cycle", TargetID: cycleID}}
	for _, item := range loaded {
		targets = append(targets, CommentTarget{TargetType: "objective", TargetID: item.objective.ID})
		for _, kr := range item.keyResults {
			targets = append(targets, CommentTarget{TargetType: "key_result", TargetID: kr.ID})
		}
	}
	progressTargets := make([][]CommentTarget, len(loaded))
	g, gctx = errgroup.WithContext(ctx)
	g.SetLimit(commentDetailConcurrency)
	for obj := range loaded {
		i := obj
		g.Go(func() error {
			var result []CommentTarget
			progresses, err := fetchCommentProgresses(gctx, runtime, "objectives", loaded[i].objective.ID)
			if err != nil {
				return err
			}
			for _, p := range progresses {
				result = append(result, CommentTarget{TargetType: "progress", TargetID: p.ID})
			}
			for _, kr := range loaded[i].keyResults {
				progresses, err := fetchCommentProgresses(gctx, runtime, "key_results", kr.ID)
				if err != nil {
					return err
				}
				for _, p := range progresses {
					result = append(result, CommentTarget{TargetType: "progress", TargetID: p.ID})
				}
			}
			progressTargets[i] = result
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	for _, group := range progressTargets {
		targets = append(targets, group...)
	}
	return targets, nil
}

// OKRCycleCommentDetail lists all comments under a cycle, grouped by target and thread.
var OKRCycleCommentDetail = common.Shortcut{
	Service: "okr", Command: "+comment-detail", Description: "List all comments under an OKR cycle, grouped by target and thread", Risk: "read", Scopes: []string{"okr:okr.comment.readonly", "okr:okr.content:readonly", "okr:okr.progress:readonly"}, AuthTypes: []string{"user", "bot"}, HasFormat: true,
	Flags: []common.Flag{
		{Name: "cycle-id", Desc: "OKR cycle ID (int64)", Required: true},
		{Name: "style", Default: "simple", Desc: "output style: simple (semi-plain content) | richtext (ContentBlock)", Enum: []string{"simple", "richtext"}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateCommentID("--cycle-id", runtime.Str("cycle-id")); err != nil {
			return err
		}
		return validateCommentStyle(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().GET("/open-apis/okr/v2/cycles/:cycle_id/objectives").Set("cycle_id", runtime.Str("cycle-id")).Desc("Fetch objectives, key results, progresses, then comments concurrently")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		targets, err := fetchCommentDetailTargets(ctx, runtime, runtime.Str("cycle-id"))
		if err != nil {
			return err
		}
		comments := make([][]Comment, len(targets))
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(commentDetailConcurrency)
		for tgt := range targets {
			i := tgt
			g.Go(func() error {
				result, err := fetchComments(gctx, runtime, targets[i])
				if err == nil {
					comments[i] = result
				}
				return err
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}
		byTarget := make(map[string][][]*RespComment, len(targets))
		for i, target := range targets {
			byTarget[target.TargetID] = responseCommentThreads(groupCommentThreads(comments[i]), runtime.Str("style"))
		}
		runtime.OutFormat(map[string]interface{}{"cycle_id": runtime.Str("cycle-id"), "comments": byTarget, "style": runtime.Str("style")}, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Found comments under cycle %s across %d target(s)\n", runtime.Str("cycle-id"), len(byTarget))
		})
		return nil
	},
}
