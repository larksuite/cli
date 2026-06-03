// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"errors"
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
)

type simpleArgs struct {
	Name  string `flag:"name" desc:"a name"`
	Count int    `flag:"count" default:"3"`
}

func TestWalkArgs_Simple(t *testing.T) {
	specs, err := walkArgs(reflect.TypeOf(&simpleArgs{}))
	if err != nil {
		t.Fatalf("walkArgs: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 field specs, got %d", len(specs))
	}
	if specs[0].FlagName != "name" || specs[1].FlagName != "count" {
		t.Errorf("flag names: %+v", specs)
	}
}

type dupTagArgs struct {
	A string `flag:"x"`
	B string `flag:"x"`
}

func TestWalkArgs_DuplicateTagPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for duplicate flag tag")
		}
	}()
	_, _ = walkArgs(reflect.TypeOf(&dupTagArgs{}))
}

type bindArgs struct {
	Name  string `flag:"name"`
	Count int    `flag:"count" default:"7"`
}

func TestRegisterAndBind_StringInt(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&bindArgs{}))
	if err := registerFlags(cmd, specs); err != nil {
		t.Fatalf("registerFlags: %v", err)
	}
	_ = cmd.ParseFlags([]string{"--name", "alice", "--count", "12"})
	out := &bindArgs{}
	if err := bindFlags(cmd, reflect.ValueOf(out).Elem(), specs); err != nil {
		t.Fatalf("bindFlags: %v", err)
	}
	if out.Name != "alice" {
		t.Errorf("Name = %q, want alice", out.Name)
	}
	if out.Count != 12 {
		t.Errorf("Count = %d, want 12", out.Count)
	}
}

func TestRegister_DefaultApplies(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&bindArgs{}))
	_ = registerFlags(cmd, specs)
	_ = cmd.ParseFlags(nil)
	out := &bindArgs{}
	_ = bindFlags(cmd, reflect.ValueOf(out).Elem(), specs)
	if out.Count != 7 {
		t.Errorf("default not applied: Count = %d, want 7", out.Count)
	}
}

type withValidatable struct {
	ID dummyValidatable `flag:"id"`
}

func TestRunValidateValue_CallsValidatable(t *testing.T) {
	v := &withValidatable{}
	specs, _ := walkArgs(reflect.TypeOf(v))
	rt := &RuntimeContext{}
	if err := runValidateValue(rt, reflect.ValueOf(v).Elem(), specs); err != nil {
		t.Errorf("runValidateValue: %v", err)
	}
}

type oneOfArgs struct {
	A *string `flag:"a"`
	B *string `flag:"b"`
}

func (oneOfArgs) OneOf() {}

type bucketArgs struct {
	Bucket oneOfArgs
}

func TestFrameworkRules_OneOfMissing(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&bucketArgs{}))
	_ = registerFlags(cmd, specs)
	_ = cmd.ParseFlags(nil)
	out := &bucketArgs{}
	_ = bindFlags(cmd, reflect.ValueOf(out).Elem(), specs)
	err := runFrameworkRules(cmd, reflect.ValueOf(out).Elem(), specs)
	if err == nil {
		t.Fatal("expected oneof_missing error")
	}
	ve := mustValidationError(t, err)
	if ve.Subtype != errs.SubtypeShortcutOneOfMissing {
		t.Errorf("Subtype = %q", ve.Subtype)
	}
}

func TestFrameworkRules_OneOfMultiple(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&bucketArgs{}))
	_ = registerFlags(cmd, specs)
	_ = cmd.ParseFlags([]string{"--a", "1", "--b", "2"})
	out := &bucketArgs{}
	_ = bindFlags(cmd, reflect.ValueOf(out).Elem(), specs)
	err := runFrameworkRules(cmd, reflect.ValueOf(out).Elem(), specs)
	if err == nil {
		t.Fatal("expected oneof_multiple error")
	}
	ve := mustValidationError(t, err)
	if ve.Subtype != errs.SubtypeShortcutOneOfMultiple {
		t.Errorf("Subtype = %q", ve.Subtype)
	}
}

// --- top-level group binding (bindGroups) ---

type dateRange struct {
	From string `flag:"from"`
	To   string `flag:"to"`
}
type groupValueArgs struct {
	Range dateRange
}

func TestBindGroups_TopLevelValueGroup(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&groupValueArgs{}))
	_ = registerFlags(cmd, specs)
	_ = cmd.ParseFlags([]string{"--from", "2026-01-01", "--to", "2026-12-31"})
	out := &groupValueArgs{}
	argsVal := reflect.ValueOf(out).Elem()
	_ = bindFlags(cmd, argsVal, specs)
	if err := bindGroups(cmd, argsVal, specs); err != nil {
		t.Fatalf("bindGroups: %v", err)
	}
	if out.Range.From != "2026-01-01" || out.Range.To != "2026-12-31" {
		t.Errorf("Range = %+v", out.Range)
	}
}

type defaultedGroup struct {
	Port string `flag:"port" default:"8080"`
	Host string `flag:"host" default:"localhost"`
}
type groupDefaultArgs struct {
	Conf defaultedGroup
}

func TestBindGroups_TopLevelValueGroup_AppliesDefaults(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&groupDefaultArgs{}))
	_ = registerFlags(cmd, specs)
	_ = cmd.ParseFlags(nil)
	out := &groupDefaultArgs{}
	argsVal := reflect.ValueOf(out).Elem()
	_ = bindFlags(cmd, argsVal, specs)
	if err := bindGroups(cmd, argsVal, specs); err != nil {
		t.Fatalf("bindGroups: %v", err)
	}
	if out.Conf.Port != "8080" || out.Conf.Host != "localhost" {
		t.Errorf("defaults not applied: Conf = %+v", out.Conf)
	}
}

type proxyConf struct {
	Host string `flag:"proxy-host"`
	Port string `flag:"proxy-port"`
}
type groupPtrArgs struct {
	Proxy *proxyConf
}

func TestBindGroups_TopLevelPtrGroup_AllocatedWhenChanged(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&groupPtrArgs{}))
	_ = registerFlags(cmd, specs)
	_ = cmd.ParseFlags([]string{"--proxy-host", "p.example.com"})
	out := &groupPtrArgs{}
	argsVal := reflect.ValueOf(out).Elem()
	_ = bindFlags(cmd, argsVal, specs)
	if err := bindGroups(cmd, argsVal, specs); err != nil {
		t.Fatalf("bindGroups: %v", err)
	}
	if out.Proxy == nil {
		t.Fatal("expected Proxy to be allocated when an inner flag was changed")
	}
	if out.Proxy.Host != "p.example.com" {
		t.Errorf("Proxy.Host = %q", out.Proxy.Host)
	}
}

func TestBindGroups_TopLevelPtrGroup_NilWhenAbsent(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&groupPtrArgs{}))
	_ = registerFlags(cmd, specs)
	_ = cmd.ParseFlags(nil)
	out := &groupPtrArgs{}
	argsVal := reflect.ValueOf(out).Elem()
	_ = bindFlags(cmd, argsVal, specs)
	if err := bindGroups(cmd, argsVal, specs); err != nil {
		t.Fatalf("bindGroups: %v", err)
	}
	if out.Proxy != nil {
		t.Errorf("expected Proxy nil when no inner flag set, got %+v", out.Proxy)
	}
}

// --- OneOf with a nested group variant (no oneof_trigger; any inner flag
// counts as attempting that variant) ---

type vidGroup struct {
	File  string `flag:"vid-file"`
	Cover string `flag:"vid-cover"`
}
type contentBucket struct {
	Text  *string `flag:"ct"`
	Video *vidGroup
}

func (contentBucket) OneOf() {}

type contentBucketArgs struct {
	Bucket contentBucket
}

func TestCheckOneOf_GroupCompanionAloneTriggersVariant(t *testing.T) {
	// Companion --vid-cover alone (no --vid-file) should count as attempting
	// the Video variant; OneOf check passes (1 variant attempted) and the
	// group completeness check then surfaces shortcut_group_incomplete with
	// the specific missing flag, not a misleading shortcut_oneof_missing.
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&contentBucketArgs{}))
	_ = registerFlags(cmd, specs)
	_ = cmd.ParseFlags([]string{"--vid-cover", "c.png"})
	out := &contentBucketArgs{}
	_ = bindFlags(cmd, reflect.ValueOf(out).Elem(), specs)
	err := runFrameworkRules(cmd, reflect.ValueOf(out).Elem(), specs)
	if err == nil {
		t.Fatal("expected group_incomplete error")
	}
	ve := mustValidationError(t, err)
	if ve.Subtype != errs.SubtypeShortcutGroupIncomplete {
		t.Errorf("Subtype = %q, want shortcut_group_incomplete", ve.Subtype)
	}
}

func TestCheckOneOf_GroupVariantBothFieldsOK(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&contentBucketArgs{}))
	_ = registerFlags(cmd, specs)
	_ = cmd.ParseFlags([]string{"--vid-file", "v.mp4", "--vid-cover", "c.png"})
	out := &contentBucketArgs{}
	_ = bindFlags(cmd, reflect.ValueOf(out).Elem(), specs)
	if err := runFrameworkRules(cmd, reflect.ValueOf(out).Elem(), specs); err != nil {
		t.Errorf("expected OK, got %v", err)
	}
}

func TestCheckOneOf_SimpleAndGroupBothAttempted_Multiple(t *testing.T) {
	// Text variant set AND Video group's companion set → both variants are
	// attempted; expect shortcut_oneof_multiple.
	cmd := &cobra.Command{Use: "test"}
	specs, _ := walkArgs(reflect.TypeOf(&contentBucketArgs{}))
	_ = registerFlags(cmd, specs)
	_ = cmd.ParseFlags([]string{"--ct", "hi", "--vid-cover", "c.png"})
	out := &contentBucketArgs{}
	_ = bindFlags(cmd, reflect.ValueOf(out).Elem(), specs)
	err := runFrameworkRules(cmd, reflect.ValueOf(out).Elem(), specs)
	if err == nil {
		t.Fatal("expected oneof_multiple error")
	}
	ve := mustValidationError(t, err)
	if ve.Subtype != errs.SubtypeShortcutOneOfMultiple {
		t.Errorf("Subtype = %q, want shortcut_oneof_multiple", ve.Subtype)
	}
}

func mustValidationError(t *testing.T, err error) *errs.ValidationError {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	return ve
}
