// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"testing"
)

// TestCalendar_QueryFreebusy tests the workflow of querying user free/busy status.
func TestCalendar_QueryFreebusy(t *testing.T) {
	// Note: +freebusy requires valid user open_id for bot identity.
	// Without a real user open_id, this workflow cannot be tested.
	t.Run("query user freebusy status", func(t *testing.T) {
		t.Skip("requires a valid user open_id (ou_xxx) for bot identity")
	})
}