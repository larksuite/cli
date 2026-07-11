// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"testing"

	"github.com/larksuite/cli/internal/core"
	lark "github.com/larksuite/oapi-sdk-go/v3"
)

// The resolver's Open host must equal the SDK's per-brand WS base URL;
// fails if the SDK constants ever drift from the resolver.
func TestWSDomainMatchesResolver(t *testing.T) {
	if got, want := core.ResolveEndpoints(core.BrandFeishu).Open, lark.FeishuBaseUrl; got != want {
		t.Errorf("feishu WS domain = %q, want SDK %q", got, want)
	}
	if got, want := core.ResolveEndpoints(core.BrandLark).Open, lark.LarkBaseUrl; got != want {
		t.Errorf("lark WS domain = %q, want SDK %q", got, want)
	}
}
