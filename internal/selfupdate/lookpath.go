// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import ("os/exec")

// execLookPath is the LookPath implementation used by VerifyBinary.
// It defaults to the standard library exec.LookPath but is swapped in tests
// via lookPathMock to provide controlled binary resolution.
var execLookPath = exec.LookPath