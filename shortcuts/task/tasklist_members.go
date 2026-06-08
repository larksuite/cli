// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var MembersTasklist = common.Shortcut{
	Service:     "task",
	Command:     "+tasklist-members",
	Description: "manage tasklist members",
	Risk:        "write",
	Scopes:      []string{"task:tasklist:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "tasklist-id", Desc: "tasklist id", Required: true},
		{Name: "set", Desc: "comma-separated open_ids to set as exact members (replaces existing)"},
		{Name: "add", Desc: "comma-separated open_ids to add as members"},
		{Name: "remove", Desc: "comma-separated open_ids to remove from members"},
	},

	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		hasSet := runtime.Str("set") != ""
		hasAdd := runtime.Str("add") != ""
		hasRemove := runtime.Str("remove") != ""

		if hasSet && (hasAdd || hasRemove) {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot combine --set with --add or --remove")
		}
		return nil
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		d := common.NewDryRunAPI()
		tlId := url.PathEscape(extractTasklistGuid(runtime.Str("tasklist-id")))

		if runtime.Str("set") != "" || (runtime.Str("add") == "" && runtime.Str("remove") == "") {
			d.Desc("GET tasklist details/members").
				GET("/open-apis/task/v2/tasklists/" + tlId).
				Params(map[string]interface{}{"user_id_type": "open_id"})
		}

		if runtime.Str("add") != "" {
			body := buildTlMembersBody(runtime.Str("add"))
			d.Desc("Add members").
				POST("/open-apis/task/v2/tasklists/" + tlId + "/add_members").
				Params(map[string]interface{}{"user_id_type": "open_id"}).
				Body(body)
		}
		if runtime.Str("remove") != "" {
			body := buildTlMembersBody(runtime.Str("remove"))
			d.Desc("Remove members").
				POST("/open-apis/task/v2/tasklists/" + tlId + "/remove_members").
				Params(map[string]interface{}{"user_id_type": "open_id"}).
				Body(body)
		}

		return d
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		tlId := url.PathEscape(extractTasklistGuid(runtime.Str("tasklist-id")))
		params := map[string]interface{}{"user_id_type": "open_id"}

		setStr := runtime.Str("set")
		addStr := runtime.Str("add")
		removeStr := runtime.Str("remove")

		// If no modifications, just list
		if setStr == "" && addStr == "" && removeStr == "" {
			data, err := callTaskAPITyped(runtime, http.MethodGet, "/open-apis/task/v2/tasklists/"+tlId, params, nil)
			if err != nil {
				return err
			}

			tl, _ := data["tasklist"].(map[string]interface{})
			membersRaw, _ := tl["members"].([]interface{})
			tlUrl, _ := tl["url"].(string)
			tlUrl = truncateTaskURL(tlUrl)

			var members []interface{}
			for _, m := range membersRaw {
				if mObj, ok := m.(map[string]interface{}); ok {
					members = append(members, map[string]interface{}{
						"id":   mObj["id"],
						"role": mObj["role"],
						"type": mObj["type"],
					})
				}
			}

			outData := map[string]interface{}{
				"guid":    tlId,
				"url":     tlUrl,
				"name":    tl["name"],
				"members": members,
			}

			runtime.OutFormat(outData, nil, func(w io.Writer) {
				fmt.Fprintf(w, "Tasklist: %s (%s)\n", tl["name"], tlId)
				if tlUrl != "" {
					fmt.Fprintf(w, "Tasklist URL: %s\n", tlUrl)
				}
				fmt.Fprintf(w, "Members (%d):\n", len(members))
				for _, m := range members {
					if mObj, ok := m.(map[string]interface{}); ok {
						fmt.Fprintf(w, "  - %s (%s)\n", mObj["id"], mObj["role"])
					}
				}
			})
			return nil
		}

		var lastTasklist map[string]interface{}
		if setStr != "" {
			// Query existing to diff for "set" behavior
			data, err := callTaskAPITyped(runtime, http.MethodGet, "/open-apis/task/v2/tasklists/"+tlId, params, nil)
			if err != nil {
				return err
			}
			lastTasklist, _ = data["tasklist"].(map[string]interface{})

			var existingIds []string
			if members, ok := lastTasklist["members"].([]interface{}); ok {
				for _, m := range members {
					if mObj, ok := m.(map[string]interface{}); ok {
						if id, ok := mObj["id"].(string); ok {
							existingIds = append(existingIds, id)
						}
					}
				}
			}

			targetIds := strings.Split(setStr, ",")
			var targetClean []string
			for _, t := range targetIds {
				t = strings.TrimSpace(t)
				if t != "" {
					targetClean = append(targetClean, t)
				}
			}

			// Diff
			var toAdd []string
			var toRemove []string

			for _, t := range targetClean {
				if !contains(existingIds, t) {
					toAdd = append(toAdd, t)
				}
			}
			for _, e := range existingIds {
				if !contains(targetClean, e) {
					toRemove = append(toRemove, e)
				}
			}

			if len(toAdd) > 0 {
				body := buildTlMembersBody(strings.Join(toAdd, ","))
				data, err := callTaskAPITyped(runtime, http.MethodPost, "/open-apis/task/v2/tasklists/"+tlId+"/add_members", params, body)
				if err != nil {
					return err
				}
				lastTasklist, _ = data["tasklist"].(map[string]interface{})
			}

			if len(toRemove) > 0 {
				body := buildTlMembersBody(strings.Join(toRemove, ","))
				data, err := callTaskAPITyped(runtime, http.MethodPost, "/open-apis/task/v2/tasklists/"+tlId+"/remove_members", params, body)
				if err != nil {
					return err
				}
				lastTasklist, _ = data["tasklist"].(map[string]interface{})
			}

		} else {
			// Add / Remove mode
			if addStr != "" {
				body := buildTlMembersBody(addStr)
				data, err := callTaskAPITyped(runtime, http.MethodPost, "/open-apis/task/v2/tasklists/"+tlId+"/add_members", params, body)
				if err != nil {
					return err
				}
				lastTasklist, _ = data["tasklist"].(map[string]interface{})
			}

			if removeStr != "" {
				body := buildTlMembersBody(removeStr)
				data, err := callTaskAPITyped(runtime, http.MethodPost, "/open-apis/task/v2/tasklists/"+tlId+"/remove_members", params, body)
				if err != nil {
					return err
				}
				lastTasklist, _ = data["tasklist"].(map[string]interface{})
			}
		}

		tlUrl, _ := lastTasklist["url"].(string)
		tlUrl = truncateTaskURL(tlUrl)

		// Standardized write output: return resource identifiers
		outData := map[string]interface{}{
			"guid": tlId,
			"url":  tlUrl,
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✅ Tasklist members updated successfully!\n")
			fmt.Fprintf(w, "Tasklist ID: %s\n", tlId)
			if tlUrl != "" {
				fmt.Fprintf(w, "Tasklist URL: %s\n", tlUrl)
			}
		})
		return nil
	},
}

func buildTlMembersBody(idsStr string) map[string]interface{} {
	ids := strings.Split(idsStr, ",")
	var members []map[string]interface{}

	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		members = append(members, map[string]interface{}{
			"id":   id,
			"role": "editor",
			"type": "user",
		})
	}

	return map[string]interface{}{
		"members": members,
	}
}
