// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"encoding/json"

	"github.com/larksuite/cli/errs"
)

type baseFormResponse struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	DisplayMode *int   `json:"display_mode,omitempty"`
	raw         json.RawMessage
}

type baseFormResponseAlias baseFormResponse

func (form *baseFormResponse) UnmarshalJSON(data []byte) error {
	var decoded baseFormResponseAlias
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*form = baseFormResponse(decoded)
	form.raw = append(form.raw[:0], data...)
	return nil
}

func (form baseFormResponse) MarshalJSON() ([]byte, error) {
	if len(form.raw) != 0 {
		return form.raw, nil
	}
	return json.Marshal(baseFormResponseAlias(form))
}

type baseFormsPageResponse struct {
	Forms     []baseFormResponse `json:"forms"`
	HasMore   bool               `json:"has_more"`
	PageToken string             `json:"page_token"`
}

type baseFormsListOutput struct {
	Forms []baseFormResponse `json:"forms"`
	Total int                `json:"total"`
}

type baseFormUpdateRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	DisplayMode *int   `json:"display_mode,omitempty"`
}

func decodeBaseFormResponse(data map[string]interface{}) (baseFormResponse, error) {
	return decodeBaseFormData[baseFormResponse](data, "form")
}

func decodeBaseFormsPageResponse(data map[string]interface{}) (baseFormsPageResponse, error) {
	return decodeBaseFormData[baseFormsPageResponse](data, "form list page")
}

func decodeBaseFormData[T any](data map[string]interface{}, responseName string) (T, error) {
	var response T
	raw, err := json.Marshal(data)
	if err != nil {
		return response, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"encode %s response for typed decoding: %v",
			responseName,
			err,
		).WithCause(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return response, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"decode %s response: %v",
			responseName,
			err,
		).WithCause(err)
	}
	return response, nil
}

func baseFormTableRow(form baseFormResponse) map[string]interface{} {
	var displayMode interface{}
	if form.DisplayMode != nil {
		displayMode = *form.DisplayMode
	}
	return map[string]interface{}{
		"id":           form.ID,
		"name":         form.Name,
		"description":  form.Description,
		"display_mode": displayMode,
	}
}
