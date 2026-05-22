// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/registry"
)

// TestGoldenEnvelopes loads every JSON file under testdata/golden/, derives
// the dotted command path from its filename, assembles the corresponding
// envelope live, and compares marshalled bytes against the file.
//
// Set UPDATE_GOLDEN=1 to overwrite goldens with the current assembly output.
// Use this when meta_data.json changes or after intentional envelope shape
// changes.
func TestGoldenEnvelopes(t *testing.T) {
	matches, err := filepath.Glob("testdata/golden/*.json")
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(matches) == 0 {
		t.Skip("no golden fixtures yet; add files to testdata/golden/")
	}
	update := os.Getenv("UPDATE_GOLDEN") == "1"

	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			dotted := strings.TrimSuffix(filepath.Base(path), ".json")
			parts := strings.Split(dotted, ".")
			if len(parts) < 3 {
				t.Fatalf("golden filename %q must be service.resource[.subres].method.json", dotted)
			}
			service := parts[0]
			methodName := parts[len(parts)-1]
			resourcePath := parts[1 : len(parts)-1]

			spec := registry.LoadFromMeta(service)
			if spec == nil {
				t.Fatalf("unknown service: %s", service)
			}
			method := findMethodInSpec(spec, resourcePath, methodName)
			if method == nil {
				t.Fatalf("method not found: %s.%s.%s", service, strings.Join(resourcePath, "."), methodName)
			}

			got := AssembleEnvelope(service, resourcePath, methodName, method)
			gotBytes, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}
			gotBytes = append(gotBytes, '\n')

			if update {
				if err := os.WriteFile(path, gotBytes, 0o644); err != nil {
					t.Fatalf("update golden failed: %v", err)
				}
				t.Logf("updated golden: %s", path)
				return
			}

			wantBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden failed: %v", err)
			}
			if string(gotBytes) != string(wantBytes) {
				t.Errorf("envelope mismatch for %s.\nHint: run `UPDATE_GOLDEN=1 go test ./internal/schema/... -run TestGoldenEnvelopes` to refresh.\n--- got ---\n%s\n--- want ---\n%s",
					dotted, gotBytes, wantBytes)
			}
		})
	}
}

// findMethodInSpec drills into a service spec by resource path and method name.
// Returns nil if not found. Handles meta_data's flat resource dotted keys
// (e.g. resource "chat.members" lives under spec.resources["chat.members"],
// not nested) as well as conventional nested forms.
func findMethodInSpec(spec map[string]interface{}, resourcePath []string, methodName string) map[string]interface{} {
	resources, _ := spec["resources"].(map[string]interface{})
	// Try joined-dot key first (matches meta_data.json's flat layout)
	joined := strings.Join(resourcePath, ".")
	if res, ok := resources[joined].(map[string]interface{}); ok {
		if methods, ok := res["methods"].(map[string]interface{}); ok {
			if m, ok := methods[methodName].(map[string]interface{}); ok {
				return m
			}
		}
	}
	// Fall back to nested form
	cur := resources
	for _, r := range resourcePath {
		resMap, ok := cur[r].(map[string]interface{})
		if !ok {
			return nil
		}
		if nested, ok := resMap["resources"].(map[string]interface{}); ok {
			cur = nested
			continue
		}
		// Last resource — look up method
		methods, _ := resMap["methods"].(map[string]interface{})
		if m, ok := methods[methodName].(map[string]interface{}); ok {
			return m
		}
		return nil
	}
	return nil
}
