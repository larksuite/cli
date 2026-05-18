// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

// FetchDriveMetaTitle looks up the document title via the drive metas batch_query API.
func FetchDriveMetaTitle(runtime *RuntimeContext, token, docType string) (string, error) {
	data, err := runtime.CallAPI(
		"POST",
		"/open-apis/drive/v1/metas/batch_query",
		nil,
		map[string]interface{}{
			"request_docs": []map[string]interface{}{
				{
					"doc_token": token,
					"doc_type":  docType,
				},
			},
		},
	)
	if err != nil {
		return "", err
	}

	metas := GetSlice(data, "metas")
	if len(metas) == 0 {
		return "", nil
	}
	meta, _ := metas[0].(map[string]interface{})
	return GetString(meta, "title"), nil
}
