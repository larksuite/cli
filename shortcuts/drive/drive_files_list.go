// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

var DriveFilesList = common.Shortcut{
	Service:     "drive",
	Command:     "+files-list",
	Description: "List files inside a Drive folder",
	Risk:        "read",
	Scopes:      []string{"drive:drive:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "folder-token", Desc: "folder token or folder URL", Required: true},
		{Name: "page-size", Type: "int", Default: "50", Desc: "page size"},
		{Name: "page-token", Desc: "pagination token for next page"},
		{Name: "order-by", Desc: "sort field (for example: EditedTime or CreateTime)"},
		{Name: "direction", Desc: "sort direction (for example: Asc or Desc)"},
		{Name: "user-id-type", Desc: "optional user ID type for owner fields"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := normalizeFolderToken(runtime.Str("folder-token"))
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		folderToken, _ := normalizeFolderToken(runtime.Str("folder-token"))
		return common.NewDryRunAPI().
			GET("/open-apis/drive/v1/files").
			Params(buildDriveFilesListDryRunParams(runtime, folderToken))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		folderToken, err := normalizeFolderToken(runtime.Str("folder-token"))
		if err != nil {
			return err
		}

		data, err := runtime.DoAPIJSON(http.MethodGet, "/open-apis/drive/v1/files", buildDriveFilesListParams(runtime, folderToken), nil)
		if err != nil {
			return err
		}

		rawFiles := extractDriveFiles(data)
		hasMore, nextPageToken := common.PaginationMeta(data)
		outData := map[string]interface{}{
			"folder_token": folderToken,
			"files":        rawFiles,
			"total":        len(rawFiles),
			"has_more":     hasMore,
			"page_token":   nextPageToken,
		}
		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(rawFiles) == 0 {
				fmt.Fprintln(w, "No files found in this folder.")
				return
			}

			rows := make([]map[string]interface{}, 0, len(rawFiles))
			for _, item := range rawFiles {
				file, _ := item.(map[string]interface{})
				rows = append(rows, map[string]interface{}{
					"name":      firstDriveString(file, "name", "title"),
					"type":      firstDriveString(file, "type"),
					"token":     firstDriveString(file, "token", "file_token"),
					"parent":    firstDriveString(file, "parent_token"),
					"edited_at": firstDriveString(file, "edit_time", "edited_time"),
					"owner_id":  pickOwnerID(file),
				})
			}
			output.PrintTable(w, rows)

			moreHint := ""
			if hasMore {
				moreHint = fmt.Sprintf(" (more available, page_token: %s)", nextPageToken)
			}
			fmt.Fprintf(w, "\n%d file(s)%s\ntip: use --format json to inspect full metadata\n", len(rawFiles), moreHint)
		})
		return nil
	},
}

func buildDriveFilesListParams(runtime *common.RuntimeContext, folderToken string) larkcore.QueryParams {
	params := larkcore.QueryParams{
		"folder_token": []string{folderToken},
		"page_size":    []string{strconv.Itoa(runtime.Int("page-size"))},
	}
	if pageToken := strings.TrimSpace(runtime.Str("page-token")); pageToken != "" {
		params["page_token"] = []string{pageToken}
	}
	if orderBy := strings.TrimSpace(runtime.Str("order-by")); orderBy != "" {
		params["order_by"] = []string{orderBy}
	}
	if direction := strings.TrimSpace(runtime.Str("direction")); direction != "" {
		params["direction"] = []string{direction}
	}
	if userIDType := strings.TrimSpace(runtime.Str("user-id-type")); userIDType != "" {
		params["user_id_type"] = []string{userIDType}
	}
	return params
}

func buildDriveFilesListDryRunParams(runtime *common.RuntimeContext, folderToken string) map[string]interface{} {
	params := map[string]interface{}{
		"folder_token": folderToken,
		"page_size":    runtime.Int("page-size"),
	}
	if pageToken := strings.TrimSpace(runtime.Str("page-token")); pageToken != "" {
		params["page_token"] = pageToken
	}
	if orderBy := strings.TrimSpace(runtime.Str("order-by")); orderBy != "" {
		params["order_by"] = orderBy
	}
	if direction := strings.TrimSpace(runtime.Str("direction")); direction != "" {
		params["direction"] = direction
	}
	if userIDType := strings.TrimSpace(runtime.Str("user-id-type")); userIDType != "" {
		params["user_id_type"] = userIDType
	}
	return params
}

func normalizeFolderToken(input string) (string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", output.ErrValidation("--folder-token must not be empty")
	}
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", output.ErrValidation("--folder-token: invalid folder URL: %v", err)
		}
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		for i := 0; i+1 < len(parts); i++ {
			if parts[i] == "folder" {
				raw = parts[i+1]
				break
			}
		}
	}
	if err := validate.ResourceName(raw, "--folder-token"); err != nil {
		return "", output.ErrValidation("%s", err)
	}
	return raw, nil
}

func extractDriveFiles(data map[string]interface{}) []interface{} {
	if files, ok := data["files"].([]interface{}); ok {
		return files
	}
	if items, ok := data["items"].([]interface{}); ok {
		return items
	}
	return nil
}

func firstDriveString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, _ := m[key].(string); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func pickOwnerID(file map[string]interface{}) string {
	owner, _ := file["owner_id"].(string)
	if strings.TrimSpace(owner) != "" {
		return owner
	}
	ownerInfo, _ := file["owner"].(map[string]interface{})
	return firstDriveString(ownerInfo, "id", "open_id", "user_id")
}
