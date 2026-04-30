// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

type driveCreateFolderSpec struct {
	Name        string
	FolderToken string
}

func newDriveCreateFolderSpec(runtime *common.RuntimeContext) driveCreateFolderSpec {
	return driveCreateFolderSpec{
		Name:        strings.TrimSpace(runtime.Str("name")),
		FolderToken: strings.TrimSpace(runtime.Str("folder-token")),
	}
}

func (s driveCreateFolderSpec) RequestBody() map[string]interface{} {
	return map[string]interface{}{
		"name":         s.Name,
		"folder_token": s.FolderToken,
	}
}

// DriveCreateFolder creates a new Drive folder under the specified parent
// folder, or under the caller's root folder when --folder-token is omitted.
var DriveCreateFolder = common.Shortcut{
	Service:     "drive",
	Command:     "+create-folder",
	Description: "Create a folder in Drive",
	Risk:        "write",
	Scopes:      []string{"space:folder:create"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "name", Desc: "folder name", Required: true},
		{Name: "folder-token", Desc: "parent folder token (default: root folder)"},
	},
	Tips: []string{
		"Omit --folder-token to create the folder in the caller's root folder.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateDriveCreateFolderSpec(newDriveCreateFolderSpec(runtime))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := newDriveCreateFolderSpec(runtime)
		dry := common.NewDryRunAPI().
			Desc("Create a folder in Drive").
			POST("/open-apis/drive/v1/files/create_folder").
			Desc("[1] Create folder").
			Body(spec.RequestBody())
		if runtime.IsBot() {
			dry.Desc("After folder creation succeeds in bot mode, the CLI will also try to grant the current CLI user full_access (可管理权限) on the new folder.")
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := newDriveCreateFolderSpec(runtime)

		target := "root folder"
		if spec.FolderToken != "" {
			target = "folder " + common.MaskToken(spec.FolderToken)
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Creating folder %q in %s...\n", spec.Name, target)

		data, err := runtime.CallAPI(
			"POST",
			"/open-apis/drive/v1/files/create_folder",
			nil,
			spec.RequestBody(),
		)
		if err != nil {
			return err
		}

		folderToken := common.GetString(data, "token")
		if folderToken == "" {
			return output.Errorf(output.ExitAPI, "api_error", "drive create_folder succeeded but returned no folder token (data.token)")
		}
		out := map[string]interface{}{
			"created":             true,
			"name":                spec.Name,
			"folder_token":        folderToken,
			"parent_folder_token": spec.FolderToken,
		}
		if url := strings.TrimSpace(common.GetString(data, "url")); url != "" {
			out["url"] = url
		} else if u := common.BuildResourceURL(runtime.Config.Brand, "folder", folderToken); u != "" {
			out["url"] = u
		}
		if grant := common.AutoGrantCurrentUserDrivePermission(runtime, folderToken, "folder"); grant != nil {
			out["permission_grant"] = grant
		}

		runtime.Out(out, nil)
		return nil
	},
}

func validateDriveCreateFolderSpec(spec driveCreateFolderSpec) error {
	if spec.Name == "" {
		return output.ErrValidation("--name must not be empty")
	}
	if nameBytes := len([]byte(spec.Name)); nameBytes > 256 {
		return output.ErrValidation("--name exceeds the maximum of 256 bytes (got %d)", nameBytes)
	}
	if spec.FolderToken != "" {
		if err := validate.ResourceName(spec.FolderToken, "--folder-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
	}
	return nil
}
