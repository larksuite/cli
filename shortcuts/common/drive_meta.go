// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

// DriveMeta is the subset of drive metas/batch_query fields used by shortcuts.
type DriveMeta struct {
	Title string
	URL   string
}

// FetchDriveMeta looks up document metadata via the drive metas batch_query API.
func FetchDriveMeta(runtime *RuntimeContext, token, docType string, withURL bool) (DriveMeta, error) {
	body := map[string]interface{}{
		"request_docs": []map[string]interface{}{
			{
				"doc_token": token,
				"doc_type":  docType,
			},
		},
	}
	if withURL {
		body["with_url"] = true
	}

	data, err := runtime.CallAPITyped(
		"POST",
		"/open-apis/drive/v1/metas/batch_query",
		nil,
		body,
	)
	if err != nil {
		return DriveMeta{}, err
	}

	metas := GetSlice(data, "metas")
	if len(metas) == 0 {
		return DriveMeta{}, nil
	}
	meta, _ := metas[0].(map[string]interface{})
	return DriveMeta{
		Title: GetString(meta, "title"),
		URL:   GetString(meta, "url"),
	}, nil
}

// FetchDriveMetaTitle looks up the document title via the drive metas batch_query API.
func FetchDriveMetaTitle(runtime *RuntimeContext, token, docType string) (string, error) {
	meta, err := FetchDriveMeta(runtime, token, docType, false)
	if err != nil {
		return "", err
	}
	return meta.Title, nil
}

// FetchDriveMetaURL looks up the document access URL via the drive metas batch_query API.
func FetchDriveMetaURL(runtime *RuntimeContext, token, docType string) (string, error) {
	meta, err := FetchDriveMeta(runtime, token, docType, true)
	if err != nil {
		return "", err
	}
	return meta.URL, nil
}
