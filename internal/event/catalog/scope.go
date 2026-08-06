// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package catalog

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"sort"
)

// SubscriptionScope returns a stable identifier scoped to (EventKey, values
// of the ParamDefs marked SubscriptionKey); the framework uses it to dedup
// preparation/cleanup gates and key per-subscription accounting. No
// SubscriptionKey params -> returns def.Key verbatim (legacy one-dimensional
// behavior).
//
// Stability contract: same EventKey + same normalized param values -> same ID
// across CLI versions; changing the encoding requires a wire-format bump.
func SubscriptionScope(def *KeyDefinition, params map[string]string) string {
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
