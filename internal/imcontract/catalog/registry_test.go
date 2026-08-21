// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package catalog

import "testing"

func TestWholeRequestPartialRecoveryContracts(t *testing.T) {
	for _, key := range []ContractKey{
		"im +feed-shortcut-create",
		"im +feed-shortcut-remove",
		"im +flag-cancel",
	} {
		contract, ok := Lookup(key)
		if !ok {
			t.Fatalf("missing contract %q", key)
		}
		if contract.PartialRecovery != PartialRecoveryWholeRequest {
			t.Fatalf("%s partial recovery = %q", key, contract.PartialRecovery)
		}
	}

	remove, _ := Lookup("im +feed-shortcut-remove")
	if remove.ReplayMode != ReplaySafe {
		t.Fatalf("feed shortcut remove replay mode = %q", remove.ReplayMode)
	}

	urgent, _ := Lookup("im messages urgent_app")
	if urgent.PartialRecovery != PartialRecoveryFailedItemsOnly {
		t.Fatalf("urgent app partial recovery = %q", urgent.PartialRecovery)
	}
}
