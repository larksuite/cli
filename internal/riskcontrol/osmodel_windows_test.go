//go:build windows

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package riskcontrol

import (
	"errors"
	"reflect"
	"testing"
)

func TestReadWindowsDeviceModelFallback(t *testing.T) {
	readError := errors.New("registry read failed")
	tests := []struct {
		name      string
		values    map[string]string
		errors    map[string]error
		want      string
		wantPaths []string
	}{
		{
			name:      "first path wins",
			values:    map[string]string{systemInfoRegistryPaths[0]: "Surface Laptop"},
			want:      "Surface Laptop",
			wantPaths: []string{systemInfoRegistryPaths[0]},
		},
		{
			name: "read failure falls back",
			errors: map[string]error{
				systemInfoRegistryPaths[0]: readError,
			},
			values: map[string]string{
				systemInfoRegistryPaths[1]: "ThinkPad X1 Carbon",
			},
			want:      "ThinkPad X1 Carbon",
			wantPaths: systemInfoRegistryPaths[:2],
		},
		{
			name: "empty normalized value falls back",
			values: map[string]string{
				systemInfoRegistryPaths[0]: " \r\n\x00",
				systemInfoRegistryPaths[1]: "Latitude 7450",
			},
			want:      "Latitude 7450",
			wantPaths: systemInfoRegistryPaths[:2],
		},
		{
			name: "all paths fail",
			errors: map[string]error{
				systemInfoRegistryPaths[0]: readError,
				systemInfoRegistryPaths[1]: readError,
				systemInfoRegistryPaths[2]: readError,
			},
			wantPaths: systemInfoRegistryPaths[:],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var paths []string
			got := readWindowsDeviceModel(func(path string) (string, error) {
				paths = append(paths, path)
				if err := tt.errors[path]; err != nil {
					return "", err
				}
				return tt.values[path], nil
			})
			if got != tt.want {
				t.Fatalf("model = %q, want %q", got, tt.want)
			}
			if !reflect.DeepEqual(paths, tt.wantPaths) {
				t.Fatalf("registry paths = %v, want %v", paths, tt.wantPaths)
			}
		})
	}
}
