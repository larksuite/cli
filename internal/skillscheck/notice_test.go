// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"sync"
	"testing"
)

func TestStaleNotice_Message(t *testing.T) {
	tests := []struct {
		name string
		n    StaleNotice
		want string
	}{
		{
			"drift",
			StaleNotice{Current: "1.0.20", Target: "1.0.21"},
			"lark-cli skills 1.0.20 out of sync with binary 1.0.21, run: lark-cli update",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.n.Message(); got != tt.want {
				t.Errorf("Message() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetGetPending(t *testing.T) {
	SetPending(nil)
	t.Cleanup(func() { SetPending(nil) })

	if got := GetPending(); got != nil {
		t.Fatalf("initial GetPending() = %+v, want nil", got)
	}

	want := &StaleNotice{Current: "1.0.20", Target: "1.0.21"}
	SetPending(want)
	got := GetPending()
	if got == nil || got.Current != "1.0.20" || got.Target != "1.0.21" {
		t.Errorf("GetPending() = %+v, want %+v", got, want)
	}
}

func TestSetGetPending_Concurrent(t *testing.T) {
	SetPending(nil)
	t.Cleanup(func() { SetPending(nil) })

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			SetPending(&StaleNotice{Current: "a", Target: "b"})
		}()
		go func() {
			defer wg.Done()
			_ = GetPending()
		}()
	}
	wg.Wait()
	// Just verifying no race; -race flag enforces.
}
