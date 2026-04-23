// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar

package cmdutil

import (
	"testing"

	sidecarcred "github.com/larksuite/cli/extension/credential/sidecar"
	sidecartrans "github.com/larksuite/cli/extension/transport/sidecar"
)

// TestIsBuiltinProvider_SidecarProviders locks the classification for the
// sidecar-mode providers enumerated in design doc §3.3.2 as "官方自带". These
// types only compile when the `authsidecar` build tag is active, so the test
// is guarded by the same tag.
func TestIsBuiltinProvider_SidecarProviders(t *testing.T) {
	cases := []struct {
		name     string
		provider any
	}{
		{"sidecar credential provider", &sidecarcred.Provider{}},
		{"sidecar transport provider", &sidecartrans.Provider{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !isBuiltinProvider(tc.provider) {
				t.Fatalf("%T must be classified as builtin (PkgPath under %s)", tc.provider, officialModulePath)
			}
		})
	}
}
