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

var driveCreateShortcutAllowedTypes = map[string]bool{
	"file":     true,
	"docx":     true,
	"bitable":  true,
	"doc":      true,
	"sheet":    true,
	"mindnote": true,
	"shortcut": true,
	"slides":   true,
}

type driveCreateShortcutSpec struct {
	FileToken   string
	FileType    string
	FolderToken string
}

func (s driveCreateShortcutSpec) RequestBody() map[string]interface{} {
	return map[string]interface{}{
		"parent_token": s.FolderToken,
		"refer_entity": map[string]interface{}{
			"token": s.FileToken,
			"type":  s.FileType,
		},
	}
}

// DriveCreateShortcut creates a Drive shortcut for an existing file in another folder.
var DriveCreateShortcut = common.Shortcut{
	Service:     "drive",
	Command:     "+create-shortcut",
	Description: "Create a Drive shortcut in another folder",
	Risk:        "write",
	Scopes:      []string{"drive:drive:write"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "source file token to reference", Required: true},
		{Name: "type", Desc: "source file type (file, docx, bitable, doc, sheet, mindnote, shortcut, slides)", Required: true},
		{Name: "folder-token", Desc: "target folder token for the new shortcut (default: root folder)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateDriveCreateShortcutSpec(driveCreateShortcutSpec{
			FileToken:   runtime.Str("file-token"),
			FileType:    strings.ToLower(runtime.Str("type")),
			FolderToken: runtime.Str("folder-token"),
		})
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := driveCreateShortcutSpec{
			FileToken:   runtime.Str("file-token"),
			FileType:    strings.ToLower(runtime.Str("type")),
			FolderToken: runtime.Str("folder-token"),
		}

		dry := common.NewDryRunAPI().
			Desc("Create a Drive shortcut")

		if spec.FolderToken == "" {
			dry.GET("/open-apis/drive/explorer/v2/root_folder/meta").
				Desc("[1] Resolve root folder token")
			spec.FolderToken = "<root_folder_token>"
		}

		step := "[1] Create shortcut"
		if spec.FolderToken == "<root_folder_token>" {
			step = "[2] Create shortcut"
		}
		dry.POST("/open-apis/drive/v1/files/create_shortcut").
			Desc(step).
			Body(spec.RequestBody())

		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := driveCreateShortcutSpec{
			FileToken:   runtime.Str("file-token"),
			FileType:    strings.ToLower(runtime.Str("type")),
			FolderToken: runtime.Str("folder-token"),
		}

		if spec.FolderToken == "" {
			fmt.Fprintf(runtime.IO().ErrOut, "No target folder specified, getting root folder...\n")
			rootToken, err := getRootFolderToken(ctx, runtime)
			if err != nil {
				return err
			}
			spec.FolderToken = rootToken
		}

		fmt.Fprintf(
			runtime.IO().ErrOut,
			"Creating shortcut for %s %s in folder %s...\n",
			spec.FileType,
			common.MaskToken(spec.FileToken),
			common.MaskToken(spec.FolderToken),
		)

		data, err := runtime.CallAPI(
			"POST",
			"/open-apis/drive/v1/files/create_shortcut",
			nil,
			spec.RequestBody(),
		)
		if err != nil {
			return err
		}

		out := map[string]interface{}{
			"created":           true,
			"source_file_token": spec.FileToken,
			"source_type":       spec.FileType,
			"folder_token":      spec.FolderToken,
		}
		if shortcutToken := firstNonEmpty(
			common.GetString(data, "shortcut_token"),
			common.GetString(data, "token"),
			common.GetString(data, "file_token"),
		); shortcutToken != "" {
			out["shortcut_token"] = shortcutToken
		}
		if url := common.GetString(data, "url"); url != "" {
			out["url"] = url
		}
		if title := common.GetString(data, "title"); title != "" {
			out["title"] = title
		}

		runtime.Out(out, nil)
		return nil
	},
}

func validateDriveCreateShortcutSpec(spec driveCreateShortcutSpec) error {
	if err := validate.ResourceName(spec.FileToken, "--file-token"); err != nil {
		return output.ErrValidation("%s", err)
	}
	if strings.TrimSpace(spec.FolderToken) != "" {
		if err := validate.ResourceName(spec.FolderToken, "--folder-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
	}
	if spec.FileType == "wiki" {
		return output.ErrValidation("unsupported file type: wiki. This shortcut only supports Drive file tokens; wiki documents must be resolved to their underlying file token first")
	}
	if spec.FileType == "folder" {
		return output.ErrValidation("unsupported file type: folder. The create_shortcut API only supports Drive files, not folders")
	}
	if !driveCreateShortcutAllowedTypes[spec.FileType] {
		return output.ErrValidation("unsupported file type: %s. Supported types: file, docx, bitable, doc, sheet, mindnote, shortcut, slides", spec.FileType)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
