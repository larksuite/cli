// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package shortcuts

import (
	"embed"
	"encoding/json"
	"sync"

	"github.com/larksuite/cli/shortcuts/common"
)

//go:embed agent_support.json
var agentSupportFS embed.FS

type agentSupportEntry struct {
	AgentSupport common.AgentSupport `json:"agentSupport"`
}

// agentSupportTable is the shipped record of which shortcuts an Agent Employee
// may run, and since CheckShortcutPrincipal it is the only thing that decides
// that access.
//
// It is maintained by hand. Each verdict is reached by taking a shortcut's
// unconditional OpenAPI dependencies and asking whether every one of them
// accepts an agent token: `allowed` when they all do, `denied` when any does
// not. updatedAt records when that reconciliation was last performed.
//
// There is no generator, and the check cannot currently be reproduced from this
// repository alone. It is settled against the registry's live accessTokens,
// while internal/registry/catalog is a published artifact refreshed in
// increments and lagging behind it -- im's v2 chats/search and the edit route
// behind `im +messages-edit` are both absent from the catalog yet decided here.
// Until a generator exists and that gap is closed, changing a verdict means
// re-checking the dependencies against the live registry and saying so in the
// commit message, because nothing in CI will catch a wrong entry. A missing
// entry is caught: every registered shortcut must have a row (see
// agentSupportGaps and its test), because an unrecorded shortcut dispatches.
type agentSupportTable struct {
	Version   int                          `json:"version"`
	UpdatedAt string                       `json:"updatedAt"`
	Commands  map[string]agentSupportEntry `json:"commands"`
}

var (
	agentSupportOnce  sync.Once
	agentSupportCache agentSupportTable
)

// loadAgentSupportTable decodes the embedded verdict table once. A malformed or
// missing file leaves every command unknown, so a broken build cannot lock an
// Agent out.
func loadAgentSupportTable() agentSupportTable {
	agentSupportOnce.Do(func() {
		agentSupportCache = agentSupportTable{Commands: map[string]agentSupportEntry{}}
		data, err := agentSupportFS.ReadFile("agent_support.json")
		if err != nil {
			return
		}
		var parsed agentSupportTable
		if json.Unmarshal(data, &parsed) != nil {
			return
		}
		if parsed.Commands == nil {
			parsed.Commands = map[string]agentSupportEntry{}
		}
		agentSupportCache = parsed
	})
	return agentSupportCache
}

// applyAgentSupport copies the recorded verdict onto matching elements of
// registered, keyed by "service command"; absent commands stay unknown. It is
// separate from stampAgentSupport so tests can drive it with a synthetic table.
func applyAgentSupport(registered []common.Shortcut, table agentSupportTable) {
	for i := range registered {
		key := registered[i].Service + " " + registered[i].Command
		if entry, ok := table.Commands[key]; ok {
			registered[i].AgentSupport = entry.AgentSupport
		}
	}
}

// stampAgentSupport applies the embedded verdict table to a build-local snapshot.
func stampAgentSupport(registered []common.Shortcut) {
	applyAgentSupport(registered, loadAgentSupportTable())
}
