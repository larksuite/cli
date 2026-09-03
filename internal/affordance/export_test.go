// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"encoding/json"

	"github.com/larksuite/cli/internal/apicatalog"
)

// Test-only conveniences: production code always holds one Resolver per build;
// the per-domain source tests only need one lookup against the registered
// content tree, so they construct a throwaway Resolver here.

func For(catalog apicatalog.Catalog, service, methodID string) (json.RawMessage, bool) {
	return NewResolver(Source(), catalog).For(service, methodID)
}

func DomainSkill(service string) (string, bool) {
	return NewResolver(Source(), apicatalog.Catalog{}).DomainSkill(service)
}

func DomainSkills(service string) ([]string, bool) {
	return NewResolver(Source(), apicatalog.Catalog{}).DomainSkills(service)
}
