//go:build darwin

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package riskcontrol

import (
	"errors"
	"reflect"
	"testing"
)

func TestReadDarwinDeviceModelPrefersProductAndFallsBackToModel(t *testing.T) {
	t.Run("product available", func(t *testing.T) {
		var keys []string
		got := readDarwinDeviceModel(func(key string) (string, error) {
			keys = append(keys, key)
			if key == "hw.product" {
				return "Mac16,1", nil
			}
			return "", errors.New("unexpected fallback")
		})
		if got != "Mac16,1" {
			t.Fatalf("model = %q, want %q", got, "Mac16,1")
		}
		if want := []string{"hw.product"}; !reflect.DeepEqual(keys, want) {
			t.Fatalf("sysctl keys = %v, want %v", keys, want)
		}
	})

	t.Run("product unavailable", func(t *testing.T) {
		var keys []string
		got := readDarwinDeviceModel(func(key string) (string, error) {
			keys = append(keys, key)
			if key == "hw.model" {
				return "MacBookPro18,3", nil
			}
			return "", errors.New("not available")
		})
		if got != "MacBookPro18,3" {
			t.Fatalf("model = %q, want %q", got, "MacBookPro18,3")
		}
		if want := []string{"hw.product", "hw.model"}; !reflect.DeepEqual(keys, want) {
			t.Fatalf("sysctl keys = %v, want %v", keys, want)
		}
	})
}
