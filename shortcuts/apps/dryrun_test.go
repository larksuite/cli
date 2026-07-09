// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import "encoding/json"

type dryRunAPICall struct {
	Method string                 `json:"method"`
	URL    string                 `json:"url"`
	Params map[string]interface{} `json:"params"`
	Body   map[string]interface{} `json:"body"`
}

type dryRunAPIEnvelope struct {
	API []dryRunAPICall
}

func (e *dryRunAPIEnvelope) UnmarshalJSON(data []byte) error {
	var raw struct {
		Data struct {
			API []dryRunAPICall `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.API = raw.Data.API
	return nil
}

func decodeDryRunDataMap(data []byte) (map[string]interface{}, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if nested, ok := raw["data"].(map[string]interface{}); ok {
		return nested, nil
	}
	return raw, nil
}
