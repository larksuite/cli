// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package draft

import (
	"encoding/json"
	"testing"
)

func TestDraftWriteBodyOptionalSeparately(t *testing.T) {
	yes, no := true, false
	for _, v := range []*bool{nil, &yes, &no} {
		raw, err := json.Marshal(draftWriteBody("eml", v))
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]interface{}
		if err = json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		value, ok := got["is_send_separately"]
		if ok != (v != nil) || (v != nil && value != *v) {
			t.Fatalf("lost setting: %s", raw)
		}
	}
}
