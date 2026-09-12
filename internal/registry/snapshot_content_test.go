// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// Help renders Catalog descriptions as plain text, so the snapshot must not
// carry markdown link syntax, "see the docs" breadcrumbs whose URL would be
// lost, or md-* markup. The registry publication strips them; this pins the
// contract so cmd/service needs no render-time heuristic for dead pointers.
var descriptionMarkup = regexp.MustCompile(`\]\(|</?md-[a-z-]+`)

func TestCatalogSnapshotDescriptionsCarryNoMarkup(t *testing.T) {
	services, err := fs.Sub(embeddedCatalogFS, "catalog/services")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(services, ".")
	if err != nil {
		t.Fatal(err)
	}
	var offenders []string
	for _, entry := range entries {
		data, err := fs.ReadFile(services, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		walkDescriptions(doc, entry.Name(), func(pointer, text string) {
			if descriptionMarkup.MatchString(text) {
				offenders = append(offenders, fmt.Sprintf("%s: %.80s", pointer, text))
			}
		})
	}
	if len(offenders) > 0 {
		t.Fatalf("%d descriptions carry markdown links or md-* markup; strip them when publishing the snapshot:\n%s",
			len(offenders), strings.Join(offenders, "\n"))
	}
}

func walkDescriptions(node any, pointer string, visit func(pointer, text string)) {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if key == "description" {
				if text, ok := child.(string); ok {
					visit(pointer+"/description", text)
					continue
				}
			}
			walkDescriptions(child, pointer+"/"+key, visit)
		}
	case []any:
		for i, child := range v {
			walkDescriptions(child, fmt.Sprintf("%s/%d", pointer, i), visit)
		}
	}
}
