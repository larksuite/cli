// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package riskcontrol

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unicode"
)

func TestHostSourceCachesNonEmptyModel(t *testing.T) {
	calls := 0
	s := &HostSource{readModel: func() string {
		calls++
		return "  MacBookPro18,3\n"
	}}

	if got := s.Snapshot(); got.ProductModel != "MacBookPro18,3" {
		t.Fatalf("first Snapshot().ProductModel = %q, want %q", got.ProductModel, "MacBookPro18,3")
	}
	if got := s.Snapshot(); got.ProductModel != "MacBookPro18,3" {
		t.Fatalf("second Snapshot().ProductModel = %q, want cached model", got.ProductModel)
	}
	if calls != 1 {
		t.Fatalf("read called %d times, want 1", calls)
	}
}

func TestHostSourceCachesEmptyModel(t *testing.T) {
	calls := 0
	s := &HostSource{readModel: func() string {
		calls++
		return ""
	}}

	if got := s.Snapshot(); got.ProductModel != "" {
		t.Fatalf("first Snapshot().ProductModel = %q, want empty", got.ProductModel)
	}
	if got := s.Snapshot(); got.ProductModel != "" {
		t.Fatalf("second Snapshot().ProductModel = %q, want cached empty result", got.ProductModel)
	}
	if calls != 1 {
		t.Fatalf("read called %d times, want 1", calls)
	}
}

func TestHostSourceReadsOnceAcrossConcurrentCalls(t *testing.T) {
	var calls atomic.Int32
	s := &HostSource{readModel: func() string {
		calls.Add(1)
		return "ThinkPad X1 Carbon"
	}}

	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			snapshot := s.Snapshot()
			if snapshot.ProductModel != "ThinkPad X1 Carbon" {
				t.Errorf("Snapshot().ProductModel = %q, want %q", snapshot.ProductModel, "ThinkPad X1 Carbon")
			}
		}()
	}
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("read called %d times, want 1", got)
	}
}

func TestNormalizeDeviceModel(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "trims surrounding whitespace", model: "  MacBookPro18,3\n", want: "MacBookPro18,3"},
		{name: "trims device tree terminator", model: "Raspberry Pi 5\x00", want: "Raspberry Pi 5"},
		{name: "allows printable Unicode", model: "联想 ThinkPad X1", want: "联想 ThinkPad X1"},
		{name: "rejects empty", model: " \t\r\n"},
		{name: "rejects invalid UTF-8", model: string([]byte{'M', 0xff, '1'})},
		{name: "removes CRLF", model: "model\r\nname", want: "modelname"},
		{name: "normalizes tab", model: "model\tname", want: "model name"},
		{name: "removes NUL", model: "model\x00name", want: "modelname"},
		{name: "removes control character", model: "model\x1fname", want: "modelname"},
		{name: "removes DEL", model: "model\x7fname", want: "modelname"},
		{name: "normalizes Unicode line separator", model: "model\u2028name", want: "model name"},
		{name: "collapses whitespace", model: "  model\t \u00a0 name  ", want: "model name"},
		{name: "accepts maximum byte length", model: strings.Repeat("a", deviceModelMaxBytes), want: strings.Repeat("a", deviceModelMaxBytes)},
		{name: "rejects overlong value", model: strings.Repeat("a", deviceModelMaxBytes+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeDeviceModel(tt.model); got != tt.want {
				t.Fatalf("normalizeDeviceModel(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestNormalizeDeviceModelRemovesHTTPControlBytes(t *testing.T) {
	for value := 0; value <= 0x7f; value++ {
		if value >= 0x20 && value < 0x7f {
			continue
		}
		t.Run(fmt.Sprintf("0x%02x", value), func(t *testing.T) {
			model := "model" + string(rune(value)) + "name"
			want := "modelname"
			if value != '\r' && value != '\n' && value != '\x00' && unicode.IsSpace(rune(value)) {
				want = "model name"
			}
			if got := normalizeDeviceModel(model); got != want {
				t.Fatalf("normalizeDeviceModel(%q) = %q, want %q", model, got, want)
			}
		})
	}
}

func TestGetOSType(t *testing.T) {
	tests := []struct {
		name string
		want OSType
	}{
		{name: "Windows", want: OSTypeWindows},
		{name: "Linux", want: OSTypeLinux},
		{name: "MacOS", want: OSTypeMacOS},
		{name: "unknown", want: OSTypeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetOSType(tt.name); got != tt.want {
				t.Errorf("GetOSType(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
