// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package agents is the top-level business layer that wires the in-repo agent
// providers into the framework registry (internal/agents). It mirrors the events
// layering: the framework/SPI lives in internal/agents, each concrete provider is
// a declarative agents.Provider value exposed by a package under agents/<scheme>/,
// and this package's init aggregates and registers them. Blank-import this
// package from cmd to populate the provider registry.
//
// To onboard a new provider: add agents/<scheme>/ exposing a Provider() value,
// then add one line to the slice below.
package agents

import (
	"github.com/larksuite/cli/agents/base"
	iagents "github.com/larksuite/cli/internal/agents"
)

func init() {
	for _, p := range []iagents.Provider{
		base.Provider(),
	} {
		iagents.Register(p)
	}
}
