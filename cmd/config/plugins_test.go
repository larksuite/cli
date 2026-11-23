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

// config plugins show must surface a plugin's EmbeddedSkills contribution in
// the rendered JSON, not only in the internal inventory struct: this command is
// the operator's window into what a fork trimmed, so the Allow/Remove/Overlay/
// Base summary has to reach stdout. Guards the render layer, which asserting the
// inventory struct alone does not exercise.
func TestConfigPluginsShow_RendersEmbeddedSkills(t *testing.T) {
	internalplatform.SetActiveInventory(&internalplatform.Inventory{
		Plugins: []internalplatform.PluginEntry{{
			Name:         "acme",
			Version:      "1.0",
			Capabilities: internalplatform.CapabilitiesView{Restricts: true, FailurePolicy: "fail-closed"},
			EmbeddedSkills: &internalplatform.SkillsOverlayView{
				Allow:   []string{"lark-im"},
				Remove:  []string{"lark-a"},
				Overlay: true,
				Base:    true,
			},
		}},
	})
	t.Cleanup(func() { internalplatform.SetActiveInventory(nil) })

	out := &bytes.Buffer{}
	f := &cmdutil.Factory{IOStreams: cmdutil.NewIOStreams(nil, out, &bytes.Buffer{})}
	if err := runConfigPluginsShow(f); err != nil {
		t.Fatalf("show: %v", err)
	}

	var got struct {
		Plugins []struct {
			EmbeddedSkills *internalplatform.SkillsOverlayView `json:"embedded_skills"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not json: %v\n%s", err, out.String())
	}
	if len(got.Plugins) != 1 {
		t.Fatalf("want 1 plugin, got %d", len(got.Plugins))
	}
	es := got.Plugins[0].EmbeddedSkills
	if es == nil {
		t.Fatalf("embedded_skills missing from rendered output:\n%s", out.String())
	}
	if len(es.Allow) != 1 || es.Allow[0] != "lark-im" ||
		len(es.Remove) != 1 || es.Remove[0] != "lark-a" ||
		!es.Overlay || !es.Base {
		t.Errorf("embedded_skills summary mismatch: %+v", es)
	}
}

// A plugin that did not customize embedded skills must not emit an
// embedded_skills key, so the field's presence is a reliable signal that a fork
// trimmed the tree.
func TestConfigPluginsShow_OmitsEmbeddedSkillsWhenAbsent(t *testing.T) {
	internalplatform.SetActiveInventory(&internalplatform.Inventory{
		Plugins: []internalplatform.PluginEntry{{
			Name:         "acme",
			Version:      "1.0",
			Capabilities: internalplatform.CapabilitiesView{Restricts: true, FailurePolicy: "fail-closed"},
		}},
	})
	t.Cleanup(func() { internalplatform.SetActiveInventory(nil) })

	out := &bytes.Buffer{}
	f := &cmdutil.Factory{IOStreams: cmdutil.NewIOStreams(nil, out, &bytes.Buffer{})}
	if err := runConfigPluginsShow(f); err != nil {
		t.Fatalf("show: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("not json: %v", err)
	}
	plugins, ok := raw["plugins"].([]any)
	if !ok || len(plugins) != 1 {
		t.Fatalf("want 1 plugin in output, got: %s", out.String())
	}
	if _, ok := plugins[0].(map[string]any)["embedded_skills"]; ok {
		t.Errorf("embedded_skills must be omitted when the plugin customized no skills; got:\n%s", out.String())
	}
}
