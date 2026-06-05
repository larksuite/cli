// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

// ResetForTesting clears the global plugin registry. Exposed for test
// isolation only — plugin authors and SDK consumers must NOT call this
// from production code. The function is exported (rather than placed in
// an internal test-only file) so that `go test ./...` works for every
// downstream package without an extra build tag.
//
// Tests that exercise plugin registration must defer
// `t.Cleanup(platform.ResetForTesting)` so subsequent tests start from a
// clean slate. The helper is NOT goroutine-safe across concurrent
// `t.Parallel()` tests — the global registry is shared process state.
func ResetForTesting() { pluginRegistry.reset() }
