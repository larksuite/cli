// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import "testing"

func TestParsePreReleaseKVs(t *testing.T) {
	data := map[string]interface{}{
		"kvs": []interface{}{
			map[string]interface{}{"key": "upload_url", "value": "https://tos/put"},
			map[string]interface{}{"key": "MIAODA_CLIENT_BASE_PATH", "value": "/app/x"},
			map[string]interface{}{"key": "", "value": "ignored"},
			"not-a-map",
		},
	}
	kvm := parsePreReleaseKVs(data)
	if kvm["upload_url"] != "https://tos/put" || kvm["MIAODA_CLIENT_BASE_PATH"] != "/app/x" {
		t.Errorf("unexpected kvm: %v", kvm)
	}
	if len(kvm) != 2 {
		t.Errorf("len = %d, want 2 (empty key and non-map entries skipped)", len(kvm))
	}
	if len(parsePreReleaseKVs(map[string]interface{}{})) != 0 {
		t.Error("empty data should yield empty map")
	}
}
