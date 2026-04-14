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
	"slides":   true,
}

type driveCreateShortcutSpec struct {
	FileToken   string
	FileType    string
	FolderToken string
}

func newDriveCreateShortcutSpec(runtime *common.RuntimeContext) driveCreateShortcutSpec {
	return driveCreateShortcutSpec{
		FileToken:   strings.TrimSpace(runtime.Str("file-token")),
		FileType:    strings.ToLower(strings.TrimSpace(runtime.Str("type"))),
		FolderToken: strings.TrimSpace(runtime.Str("folder-token")),
	}
}

// RequestBody builds the create_shortcut API payload from the shortcut spec.
func (s driveCreateShortcutSpec) RequestBody() map[string]interface{} {
	return map[string]interface{}{
		"parent_token": s.FolderToken,
		"refer_entity": map[string]interface{}{
			"refer_token": s.FileToken,
			"refer_type":  s.FileType,
		},
	}
}

// DriveCreateShortcut creates a Drive shortcut for an existing file in another folder.
var DriveCreateShortcut = common.Shortcut{
	Service:     "drive",
	Command:     "+create-shortcut",
	Description: "Create a Drive shortcut in another folder",
	Risk:        "write",
	Scopes:      []string{"space:document:shortcut"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "source file token to reference", Required: true},
		{Name: "type", Desc: "source file type (file, docx, bitable, doc, sheet, mindnote, slides)", Required: true},
		{Name: "folder-token", Desc: "target folder token for the new shortcut", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateDriveCreateShortcutSpec(newDriveCreateShortcutSpec(runtime))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := newDriveCreateShortcutSpec(runtime)

		return common.NewDryRunAPI().
			Desc("Create a Drive shortcut").
			POST("/open-apis/drive/v1/files/create_shortcut").
			Desc("[1] Create shortcut").
			Body(spec.RequestBody())
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := newDriveCreateShortcutSpec(runtime)

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
		if shortcutToken := common.GetString(data, "succ_shortcut_node", "token"); shortcutToken != "" {
			out["shortcut_token"] = shortcutToken
		}
		if url := common.GetString(data, "succ_shortcut_node", "url"); url != "" {
			out["url"] = url
		}
		if title := common.GetString(data, "succ_shortcut_node", "name"); title != "" {
			out["title"] = title
		}

		runtime.Out(out, nil)
		return nil
	},
}

// validateDriveCreateShortcutSpec validates shortcut creation inputs before API execution.
func validateDriveCreateShortcutSpec(spec driveCreateShortcutSpec) error {
	if err := validate.ResourceName(spec.FileToken, "--file-token"); err != nil {
		return output.ErrValidation("%s", err)
	}
	if err := validate.ResourceName(spec.FolderToken, "--folder-token"); err != nil {
		return output.ErrValidation("%s", err)
	}
	if spec.FileType == "wiki" {
		return output.ErrValidation("unsupported file type: wiki. This shortcut only supports Drive file tokens; wiki documents must be resolved to their underlying file token first")
	}
	if spec.FileType == "folder" {
		return output.ErrValidation("unsupported file type: folder. The create_shortcut API only supports Drive files, not folders")
	}
	if !driveCreateShortcutAllowedTypes[spec.FileType] {
		return output.ErrValidation("unsupported file type: %s. Supported types: file, docx, bitable, doc, sheet, mindnote, slides", spec.FileType)
	}
	return nil
}
