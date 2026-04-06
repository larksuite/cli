// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import "strings"

const DomainUnknown = "unknown"

var domainPrefixes = []struct {
	prefix string
	domain string
}{
	{prefix: "im.", domain: "im"},
	{prefix: "base.", domain: "base"},
	{prefix: "bitable.", domain: "base"},
	{prefix: "docs.", domain: "docs"},
	{prefix: "docx.", domain: "docs"},
	{prefix: "drive.", domain: "docs"},
	{prefix: "calendar.", domain: "calendar"},
	{prefix: "task.", domain: "task"},
	{prefix: "contact.", domain: "contact"},
	{prefix: "vc.", domain: "vc"},
}

// ResolveDomain maps a normalized event to a routing domain based on its raw event type prefix.
func ResolveDomain(evt *Event) string {
	if evt == nil || evt.EventType == "" {
		return DomainUnknown
	}

	for _, mapping := range domainPrefixes {
		if strings.HasPrefix(evt.EventType, mapping.prefix) {
			return mapping.domain
		}
	}

	return DomainUnknown
}
