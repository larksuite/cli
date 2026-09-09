// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const driveQueryByTokenPath = "/open-apis/drive/v2/files/query_by_token"

type driveDownloadObject struct {
	ObjToken    string `json:"obj_token"`
	ObjType     string `json:"obj_type"`
	IsWikiToken bool   `json:"is_wiki_token"`
}

// resolveDriveDownloadSource treats entity lookup as a best-effort enhancement.
// Only a successful, usable response replaces the original resolution path.
func resolveDriveDownloadSource(ctx context.Context, runtime *common.RuntimeContext, source driveFileSource) (string, driveFileWikiResolution, error) {
	token := source.FileToken
	if source.NeedsWikiResolution() {
		token = source.WikiToken
	}
	object, err := queryDriveDownloadObject(runtime, token)
	if err == nil {
		// A successful lookup is authoritative about the object type.
		// Online documents still require +export, not the file download API.
		if object.ObjType != "file" {
			return "", driveFileWikiResolution{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"token resolved to %q, but download only supports uploaded Drive files",
				object.ObjType,
			).WithParam(source.InputParam).
				WithHint("for doc/docx/sheet/bitable/slides documents, use drive +export instead")
		}
		var resolution driveFileWikiResolution
		if object.IsWikiToken {
			resolution = driveFileWikiResolution{
				Resolved: true, WikiToken: token,
				ObjToken: object.ObjToken, ObjType: object.ObjType,
			}
		}
		return object.ObjToken, resolution, nil
	}

	fmt.Fprintf(runtime.IO().ErrOut, "warning: token lookup failed; using original download resolution: %v\n", err)
	if source.NeedsWikiResolution() {
		if err := runtime.EnsureScopes([]string{driveWikiNodeRetrieveScope}); err != nil {
			return "", driveFileWikiResolution{}, err
		}
		return resolveDriveFileWikiSource(ctx, runtime, source)
	}
	return source.FileToken, driveFileWikiResolution{}, nil
}

func queryDriveDownloadObject(runtime *common.RuntimeContext, token string) (driveDownloadObject, error) {
	data, err := runtime.CallAPITyped("GET", driveQueryByTokenPath, map[string]interface{}{"token": token}, nil)
	if err != nil {
		return driveDownloadObject{}, err
	}
	var object driveDownloadObject
	raw, err := json.Marshal(data)
	if err == nil {
		err = json.Unmarshal(raw, &object)
	}
	if err != nil {
		return driveDownloadObject{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "token lookup returned invalid object data").WithCause(err)
	}
	if object.ObjToken == "" || object.ObjType == "" {
		return driveDownloadObject{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "token lookup returned incomplete object data")
	}
	if err := validate.ResourceName(object.ObjToken, "obj_token"); err != nil {
		return driveDownloadObject{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "token lookup returned an invalid object token").WithCause(err)
	}
	// The response status describes the input node, not the underlying object.
	// Authorization and download remain responsible for resource availability.
	return object, nil
}
