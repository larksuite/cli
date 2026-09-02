// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

import (
	"context"
	"testing"
)

// legacyRegistrarFake is a downstream Registrar implementation written
// BEFORE EmbeddedSkills existed. It must keep compiling: doc.go declares
// every exported symbol a stability contract, so Registrar can never widen.
// If this file goes red, a method was added to Registrar -- move it to an
// optional extension interface instead (see EmbeddedSkillsRegistrar).
type legacyRegistrarFake struct{}

func (legacyRegistrarFake) Observe(When, string, Selector, Observer)    {}
func (legacyRegistrarFake) Wrap(string, Selector, Wrapper)              {}
func (legacyRegistrarFake) On(LifecycleEvent, string, LifecycleHandler) {}
func (legacyRegistrarFake) Restrict(*Rule)                              {}

var _ Registrar = legacyRegistrarFake{}

// skillsRegistrarFake opts into the optional extension.
type skillsRegistrarFake struct {
	legacyRegistrarFake
	got *SkillsOverlay
}

func (f *skillsRegistrarFake) EmbeddedSkills(spec *SkillsOverlay) { f.got = spec }

var _ EmbeddedSkillsRegistrar = (*skillsRegistrarFake)(nil)

// Drive the legacy surface through the interface so the fake stays a
// faithful stand-in for downstream usage (and none of it reads as dead code).
func TestLegacyRegistrarFake_ImplementsContractSurface(t *testing.T) {
	var r Registrar = legacyRegistrarFake{}
	r.Observe(Before, "x.obs", All(), func(context.Context, Invocation) {})
	r.Wrap("x.wrap", All(), func(next Handler) Handler { return next })
	r.On(Startup, "x.boot", func(context.Context, *LifecycleContext) error { return nil })
	r.Restrict(&Rule{Deny: []string{"config/**"}})
}

// A plugin that declared EmbeddedSkills fails closed against a host whose
// registrar lacks the optional extension, and succeeds against one that has it.
func TestBuiltPlugin_embeddedSkillsRequiresOptionalInterface(t *testing.T) {
	p := NewPlugin("acme", "1.0").
		EmbeddedSkills(&SkillsOverlay{Remove: []string{"lark-a"}}).
		MustBuild()

	if err := p.Install(legacyRegistrarFake{}); err == nil {
		t.Error("Install must fail closed when the host cannot honour EmbeddedSkills")
	}

	host := &skillsRegistrarFake{}
	if err := p.Install(host); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if host.got == nil || len(host.got.Remove) != 1 || host.got.Remove[0] != "lark-a" {
		t.Errorf("SkillsOverlay must reach the opted-in host, got %+v", host.got)
	}
}
