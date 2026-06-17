// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"sort"

	"github.com/larksuite/cli/internal/event"
)

// ComputeSubscriptionID returns a stable identifier scoped to (EventKey, values
// of the ParamDefs marked SubscriptionKey); the framework uses it to dedup
// PreConsume/cleanup gates and key Hub counts per-subscription. No SubscriptionKey
// params -> returns def.Key verbatim (legacy one-dimensional behavior).
//
// Stability contract: same EventKey + same normalized param values -> same ID
// across CLI versions; changing the encoding requires a wire-format bump.
func ComputeSubscriptionID(def *event.KeyDefinition, params map[string]string) string {
	type kv struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	var subParams []kv
	for _, p := range def.Params {
		if !p.SubscriptionKey {
			continue
		}
		subParams = append(subParams, kv{Name: p.Name, Value: params[p.Name]})
	}
	if len(subParams) == 0 {
		return def.Key
	}
	sort.Slice(subParams, func(i, j int) bool { return subParams[i].Name < subParams[j].Name })
	raw, _ := json.Marshal(subParams) // err impossible: kv has no unmarshalable fields
	sum := sha256.Sum256(raw)
	return def.Key + ":" + base64.RawURLEncoding.EncodeToString(sum[:12])
}
