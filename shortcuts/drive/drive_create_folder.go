// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func buildCreateFolderBody(runtime *common.RuntimeContext) (string, string, map[string]interface{}) {
	name := strings.TrimSpace(runtime.Str("name"))
	folderToken := strings.TrimSpace(runtime.Str("folder-token"))
	body := map[string]interface{}{"name": name}
	if folderToken != "" {
		body["folder_token"] = folderToken
	}
	return name, folderToken, body
}

// DriveCreateFolder creates a folder in Drive.
var DriveCreateFolder = common.Shortcut{
	Service:     "drive",
	Command:     "+create-folder",
	Description: "Create a folder in Drive",
	Risk:        "write",
	Scopes:      []string{"drive:drive"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "name", Desc: "folder name", Required: true},
		{Name: "folder-token", Desc: "parent folder token (default: root)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("name")) == "" {
			return output.ErrValidation("--name cannot be empty")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		_, _, body := buildCreateFolderBody(runtime)
		return common.NewDryRunAPI().
			POST("/open-apis/drive/v1/files/create_folder").
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		name, folderToken, body := buildCreateFolderBody(runtime)
		data, err := runtime.DoAPIJSON(
			"POST",
			"/open-apis/drive/v1/files/create_folder",
			nil,
			body,
		)
		if err != nil {
			return err
		}
		token := common.GetString(data, "token")
		url := common.GetString(data, "url")
		out := map[string]interface{}{
			"token": token,
			"url":   url,
			"name":  name,
		}
		if folderToken != "" {
			out["folder_token"] = folderToken
		}
		runtime.Out(out, nil)
		return nil
	},
}
