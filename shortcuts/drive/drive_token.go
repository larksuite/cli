// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const driveMetadataReadScope = "drive:drive.metadata:readonly"

const driveQueryByTokenPath = "/open-apis/drive/v2/files/query_by_token"

type driveTokenInfo struct {
	ObjToken    string `json:"obj_token"`
	ObjType     string `json:"obj_type"`
	IsWikiToken bool   `json:"is_wiki_token"`
}

func queryDriveTokenInfo(runtime *common.RuntimeContext, token string) (driveTokenInfo, error) {
	data, err := runtime.CallAPITyped("GET", driveQueryByTokenPath, map[string]interface{}{"token": token}, nil)
	if err != nil {
		return driveTokenInfo{}, err
	}
	var object driveTokenInfo
	raw, err := json.Marshal(data)
	if err == nil {
		err = json.Unmarshal(raw, &object)
	}
	if err != nil {
		return driveTokenInfo{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "token lookup returned invalid object data").WithCause(err)
	}
	if object.ObjToken == "" || object.ObjType == "" {
		return driveTokenInfo{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "token lookup returned incomplete object data")
	}
	if err := validate.ResourceName(object.ObjToken, "obj_token"); err != nil {
		return driveTokenInfo{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "token lookup returned an invalid object token").WithCause(err)
	}
	// The response status describes the input node, not the underlying object.
	// The file operation remains responsible for resource availability.
	return object, nil
}
