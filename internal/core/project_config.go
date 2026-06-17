// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	// ProjectConfigDirName is the project-local CLI config directory.
	ProjectConfigDirName = ".lark-cli"
	// ProjectConfigFileName is the project-local CLI config file.
	ProjectConfigFileName = "config.json"
)

// ProjectConfig is intentionally small: it stores project preferences, not
// credentials or login state.
type ProjectConfig struct {
	Profile string `json:"profile,omitempty"`
}

// ProjectProfileBinding is the resolved project profile and its source file.
type ProjectProfileBinding struct {
	Profile string
	Path    string
}

// ResolveProjectProfile finds and parses the nearest project profile binding.
func ResolveProjectProfile() (*ProjectProfileBinding, error) {
	cwd, err := vfs.Getwd()
	if err != nil {
		return nil, &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("cannot determine working directory: %v", err)}
	}
	return ResolveProjectProfileFrom(cwd)
}

// ResolveProjectProfileFrom finds and parses the nearest project profile binding
// from startDir upward. Search stops after the nearest Git root is checked.
func ResolveProjectProfileFrom(startDir string) (*ProjectProfileBinding, error) {
	path, ok, err := FindProjectConfigPath(startDir)
	if err != nil || !ok {
		return nil, err
	}
	cfg, err := LoadProjectConfig(path)
	if err != nil {
		return nil, err
	}
	return &ProjectProfileBinding{Profile: cfg.Profile, Path: path}, nil
}

// ProjectConfigPath returns the project config path under dir.
func ProjectConfigPath(dir string) string {
	return filepath.Join(dir, ProjectConfigDirName, ProjectConfigFileName)
}

// FindProjectConfigPath returns the nearest .lark-cli/config.json from startDir upward.
func FindProjectConfigPath(startDir string) (string, bool, error) {
	dir := filepath.Clean(startDir)
	for {
		path := ProjectConfigPath(dir)
		info, err := vfs.Stat(path)
		switch {
		case err == nil && info.IsDir():
			return "", false, &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("project config %s is a directory", path)}
		case err == nil:
			return path, true, nil
		case !errors.Is(err, os.ErrNotExist):
			return "", false, &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("cannot inspect project config %s: %v", path, err)}
		}

		if isGitRoot(dir) {
			return "", false, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}

// ProjectConfigWritePath returns where profile bind should write. It updates an
// existing binding when found; otherwise it writes at the Git root, or cwd when
// outside a Git repository.
func ProjectConfigWritePath(startDir string) (string, error) {
	if path, ok, err := FindProjectConfigPath(startDir); err != nil || ok {
		return path, err
	}
	root := findGitRoot(startDir)
	if root == "" {
		root = filepath.Clean(startDir)
	}
	return ProjectConfigPath(root), nil
}

// LoadProjectConfig parses a project-local config file.
func LoadProjectConfig(path string) (*ProjectConfig, error) {
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("cannot read project config %s: %v", path, err)}
	}
	var cfg ProjectConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("invalid project config %s: %v", path, err)}
	}
	cfg.Profile = strings.TrimSpace(cfg.Profile)
	if cfg.Profile == "" {
		return nil, &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("project config %s must set profile", path)}
	}
	if err := ValidateProfileName(cfg.Profile); err != nil {
		return nil, &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("invalid project profile in %s: %v", path, err)}
	}
	return &cfg, nil
}

// SaveProjectConfig writes the minimal project profile binding.
func SaveProjectConfig(path, profile string) error {
	profile = strings.TrimSpace(profile)
	if err := ValidateProfileName(profile); err != nil {
		return &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("invalid project profile: %v", err)}
	}
	fields := map[string]json.RawMessage{}
	data, err := vfs.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &fields); err != nil {
			return &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("invalid project config %s: %v", path, err)}
		}
	case !errors.Is(err, os.ErrNotExist):
		return &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("cannot read project config %s: %v", path, err)}
	}
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("failed to marshal project config: %v", err)}
	}
	fields["profile"] = profileJSON
	data, err = json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("failed to marshal project config: %v", err)}
	}
	if err := vfs.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("cannot create project config directory %s: %v", filepath.Dir(path), err)}
	}
	if err := validate.AtomicWrite(path, append(data, '\n'), 0600); err != nil {
		return &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("cannot write project config %s: %v", path, err)}
	}
	return nil
}

// RemoveProjectProfile removes only the profile binding. If no other fields
// remain, the project config file is deleted.
func RemoveProjectProfile(path string) (fileRemoved bool, profileRemoved bool, err error) {
	data, err := vfs.ReadFile(path)
	if err != nil {
		return false, false, &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("cannot read project config %s: %v", path, err)}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return false, false, &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("invalid project config %s: %v", path, err)}
	}
	if _, ok := fields["profile"]; !ok {
		return false, false, nil
	}
	delete(fields, "profile")
	if len(fields) == 0 {
		if err := vfs.Remove(path); err != nil {
			return false, false, &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("failed to remove project config %s: %v", path, err)}
		}
		if dir := filepath.Dir(path); filepath.Base(dir) == ProjectConfigDirName {
			_ = vfs.Remove(dir)
		}
		return true, true, nil
	}
	next, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return false, false, &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("failed to marshal project config: %v", err)}
	}
	if err := validate.AtomicWrite(path, append(next, '\n'), 0600); err != nil {
		return false, false, &ConfigError{Code: 3, Type: "config", Message: fmt.Sprintf("cannot write project config %s: %v", path, err)}
	}
	return false, true, nil
}

// ProjectProfileNotFoundError explains that a project binding references a
// profile that is not present in the user's global profile list.
func ProjectProfileNotFoundError(profile, path string, names []string) error {
	hint := "run: lark-cli profile list"
	if path != "" {
		hint = fmt.Sprintf("project config: %s; %s", path, hint)
	}
	if len(names) > 0 {
		hint += fmt.Sprintf("; available profiles: %s", formatProfileNames(names))
	}
	return &ConfigError{
		Code:    3,
		Type:    "config",
		Message: fmt.Sprintf("profile %q is configured by project but not found", profile),
		Hint:    hint,
	}
}

func isGitRoot(dir string) bool {
	_, err := vfs.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func findGitRoot(startDir string) string {
	dir := filepath.Clean(startDir)
	for {
		if isGitRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
