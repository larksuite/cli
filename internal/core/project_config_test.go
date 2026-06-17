// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeProjectConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, ProjectConfigFileName)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func TestResolveProjectProfileFrom_FindsNearestConfig(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	writeProjectConfig(t, repo, `{"profile":"root"}`)
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatalf("MkdirAll(sub): %v", err)
	}
	subConfig := writeProjectConfig(t, sub, `{"profile":"child"}`)

	got, err := ResolveProjectProfileFrom(filepath.Join(sub, "deep"))
	if err != nil {
		t.Fatalf("ResolveProjectProfileFrom() error = %v", err)
	}
	if got.Profile != "child" || got.Path != subConfig {
		t.Fatalf("binding = %#v, want child at %s", got, subConfig)
	}
}

func TestFindProjectConfigPath_StopsAtGitRoot(t *testing.T) {
	parent := t.TempDir()
	writeProjectConfig(t, parent, `{"profile":"parent"}`)
	repo := filepath.Join(parent, "repo")
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("MkdirAll(.git): %v", err)
	}
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatalf("MkdirAll(sub): %v", err)
	}

	_, ok, err := FindProjectConfigPath(sub)
	if err != nil {
		t.Fatalf("FindProjectConfigPath() error = %v", err)
	}
	if ok {
		t.Fatal("FindProjectConfigPath() found config above Git root")
	}
}

func TestProjectConfigWritePath_UsesGitRootWhenNoExistingConfig(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatalf("MkdirAll(sub): %v", err)
	}

	got, err := ProjectConfigWritePath(sub)
	if err != nil {
		t.Fatalf("ProjectConfigWritePath() error = %v", err)
	}
	want := filepath.Join(repo, ProjectConfigFileName)
	if got != want {
		t.Fatalf("ProjectConfigWritePath() = %q, want %q", got, want)
	}
}

func TestLoadProjectConfig_InvalidJSONFailsClosed(t *testing.T) {
	path := writeProjectConfig(t, t.TempDir(), `{`)
	_, err := LoadProjectConfig(path)
	if err == nil {
		t.Fatal("LoadProjectConfig() error = nil, want invalid config error")
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *ConfigError", err)
	}
	if cfgErr.Message == "" {
		t.Fatal("ConfigError.Message is empty")
	}
}

func TestLoadProjectConfig_MissingProfileFailsClosed(t *testing.T) {
	path := writeProjectConfig(t, t.TempDir(), `{"defaults":{"appId":"cli_x"}}`)
	_, err := LoadProjectConfig(path)
	if err == nil {
		t.Fatal("LoadProjectConfig() error = nil, want missing profile error")
	}
}

func TestSaveProjectConfig_PreservesOtherFields(t *testing.T) {
	path := writeProjectConfig(t, t.TempDir(), `{"defaults":{"appId":"cli_x"}}`)
	if err := SaveProjectConfig(path, "bytedance"); err != nil {
		t.Fatalf("SaveProjectConfig() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(project config): %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("Unmarshal(project config): %v", err)
	}
	if _, ok := fields["defaults"]; !ok {
		t.Fatalf("defaults field missing after save: %s", string(data))
	}
	var profile string
	if err := json.Unmarshal(fields["profile"], &profile); err != nil {
		t.Fatalf("Unmarshal(profile): %v", err)
	}
	if profile != "bytedance" {
		t.Fatalf("profile = %q, want bytedance", profile)
	}
}

func TestRemoveProjectProfile_PreservesOtherFields(t *testing.T) {
	path := writeProjectConfig(t, t.TempDir(), `{"profile":"bytedance","defaults":{"appId":"cli_x"}}`)
	fileRemoved, profileRemoved, err := RemoveProjectProfile(path)
	if err != nil {
		t.Fatalf("RemoveProjectProfile() error = %v", err)
	}
	if fileRemoved || !profileRemoved {
		t.Fatalf("RemoveProjectProfile() = fileRemoved:%v profileRemoved:%v, want file kept and profile removed", fileRemoved, profileRemoved)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(project config): %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("Unmarshal(project config): %v", err)
	}
	if _, ok := fields["defaults"]; !ok {
		t.Fatalf("defaults field missing after profile removal: %s", string(data))
	}
	if _, ok := fields["profile"]; ok {
		t.Fatalf("profile field still present after removal: %s", string(data))
	}
	if _, err := LoadProjectConfig(path); err == nil {
		t.Fatal("LoadProjectConfig() error = nil after profile removal, want missing profile")
	}
}

func TestValidateProfileName_AllowsHyphenatedNames(t *testing.T) {
	for _, name := range []string{"bytedance", "team-prod", "lark-boe"} {
		if err := ValidateProfileName(name); err != nil {
			t.Fatalf("ValidateProfileName(%q) error = %v", name, err)
		}
	}
}
