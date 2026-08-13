// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

// Provider packages are pure data (no init side effect); the top-level agents
// package's init aggregates and registers them. In production that package is
// blank-imported from cmd/build.go, not by cmd/agents. A few tests here assert
// against a real registered provider (base), so blank-import the top-level
// agents package to run its registration for the test binary.
import _ "github.com/larksuite/cli/agents"
