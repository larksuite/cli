// internal/event/consume/fingerprint.go
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
// of all ParamDef entries with SubscriptionKey=true). Used by the framework
// to dedup PreConsume/cleanup gates and route Hub keyCounts per-subscription.
//
// Algorithm:
//  1. Collect [name, value] pairs for every ParamDef with SubscriptionKey=true.
//     Values come from the params map; missing keys are treated as empty string.
//  2. Sort the pairs by Name to make the result order-independent of ParamDef
//     declaration order.
//  3. Canonical JSON-encode the sorted list.
//  4. sha256, truncate to 12 bytes (96 bits), base64URL without padding (16 chars).
//  5. Return "<EventKey>:<16-char fingerprint>".
//
// Degenerate case: no params are SubscriptionKey -> return def.Key verbatim
// (matches today's one-dimensional behavior; backward-compatible with legacy daemons).
//
// Stability contract: same EventKey + same param values (after caller-side
// normalization) -> same SubscriptionID across CLI versions. Changing this
// algorithm requires a wire-format version bump.
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
