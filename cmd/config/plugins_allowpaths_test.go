// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	internalplatform "github.com/larksuite/cli/internal/platform"
)

// config plugins show is the operator's window into what a distribution
// widened: a plugin's AllowPaths declaration must reach stdout as declared,
// and a plugin without one must not carry the key, so its presence is a
// reliable signal.
func TestConfigPluginsShow_RendersAllowPathsOnlyWhenDeclared(t *testing.T) {
	internalplatform.SetActiveInventory(&internalplatform.Inventory{
		Plugins: []internalplatform.PluginEntry{
			{
				Name:         "acme",
				Version:      "1.0",
				Capabilities: internalplatform.CapabilitiesView{FailurePolicy: "fail-closed"},
				AllowPaths:   []string{"/data/exports", "~/Downloads/acme"},
			},
			{
				Name:         "audit",
				Version:      "1.0",
				Capabilities: internalplatform.CapabilitiesView{FailurePolicy: "fail-open"},
			},
		},
	})
	t.Cleanup(func() { internalplatform.SetActiveInventory(nil) })

	out := &bytes.Buffer{}
	f := &cmdutil.Factory{IOStreams: cmdutil.NewIOStreams(nil, out, &bytes.Buffer{})}
	if err := runConfigPluginsShow(f); err != nil {
		t.Fatalf("show: %v", err)
	}

	var got struct {
		Plugins []map[string]json.RawMessage `json:"plugins"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not json: %v\n%s", err, out.String())
	}
	if len(got.Plugins) != 2 {
		t.Fatalf("want 2 plugins, got %d", len(got.Plugins))
	}

	var roots []string
	if err := json.Unmarshal(got.Plugins[0]["allow_paths"], &roots); err != nil {
		t.Fatalf("allow_paths missing or malformed for acme:\n%s", out.String())
	}
	if len(roots) != 2 || roots[0] != "/data/exports" || roots[1] != "~/Downloads/acme" {
		t.Errorf("allow_paths = %v, want the declared roots in order", roots)
	}
	if _, present := got.Plugins[1]["allow_paths"]; present {
		t.Errorf("audit declared no allow paths but the key was rendered:\n%s", out.String())
	}
}
