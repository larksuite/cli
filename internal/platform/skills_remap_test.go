// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package internalplatform

import (
	"testing"

	"github.com/larksuite/cli/extension/platform"
)

func TestStagingRegistrarSnapshotsSkillReferenceRemaps(t *testing.T) {
	remaps := []platform.SkillRefRemap{
		platform.RemapSkillRef("lark-doc", "acme-docx"),
	}
	spec := &platform.SkillsOverlay{ReferenceRemaps: remaps}
	r := newStagingRegistrar("acme")
	r.EmbeddedSkills(spec)

	remaps[0] = platform.RemapSkillRef("lark-doc", "mutated")
	spec.ReferenceRemaps = nil

	if r.skillsOverlay == nil || len(r.skillsOverlay.ReferenceRemaps) != 1 {
		t.Fatalf("staged remaps = %+v, want one snapshotted mapping", r.skillsOverlay)
	}
	got := r.skillsOverlay.ReferenceRemaps[0]
	if got.From() != "lark-doc" || got.To() != "acme-docx" {
		t.Fatalf("staged remap = %q -> %q, want lark-doc -> acme-docx", got.From(), got.To())
	}
}

func TestStagingRegistrarRejectsFailOpenEmbeddedSkills(t *testing.T) {
	for _, spec := range []*platform.SkillsOverlay{
		{Remove: []string{"lark-doc"}},
		nil,
	} {
		r := newStagingRegistrar("acme")
		r.EmbeddedSkills(spec)

		err := r.validateSelf(platform.Capabilities{FailurePolicy: platform.FailOpen})
		if err == nil {
			t.Fatalf("validateSelf accepted FailOpen+EmbeddedSkills(%+v)", spec)
		}
		pi, ok := err.(*PluginInstallError)
		if !ok || pi.ReasonCode != ReasonInvalidCapability {
			t.Fatalf("err = %v, want PluginInstallError reason_code %q", err, ReasonInvalidCapability)
		}
	}
}
