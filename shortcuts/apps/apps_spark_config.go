// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// sparkJSONRelPath is the project declaration file of the artifact-hosting
// protocol (妙搭产物托管协议规范 §3): how to dev/build, plus the app state
// section written back by the deploy chain.
const sparkJSONRelPath = "spark.json"

// appDevDefaultBuildOutput is the protocol default for build.output (the
// same-origin artifact directory). Since protocol v0.3 there is no default
// build command: a missing build.command means buildless (pack the output
// directory as-is).
const appDevDefaultBuildOutput = "dist/output"

// appDevProjectConfig is the resolved view of the project declaration that
// +deploy consumes. Fields are filled with protocol defaults when
// the declaration omits them.
type appDevProjectConfig struct {
	Stack   string
	Version string
	// BuildCommand is nil for buildless projects (no build.command declared):
	// the output directory is packed as-is.
	BuildCommand []string
	// BuildOutput is the same-origin artifact directory (protocol default
	// dist/output).
	BuildOutput string
	// BuildOutputCDN is the CDN artifact directory; empty means Level 1
	// (no CDN split).
	BuildOutputCDN string
	AppID          string
	AppURL         string
}

// sparkJSONDoc mirrors the spark.json schema (§3). Unknown fields are
// ignored on read and preserved on write (the writer re-marshals the raw
// map, not this struct).
type sparkJSONDoc struct {
	Stack   string `json:"stack"`
	Version string `json:"version"`
	Build   struct {
		Command   []string `json:"command"`
		Output    string   `json:"output"`
		OutputCDN string   `json:"output_cdn"`
	} `json:"build"`
	App struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"app"`
}

// readAppDevProjectConfig loads the project declaration from
// <dir>/spark.json. found=false means the file does not exist — the
// directory is not a Miaoda app project.
func readAppDevProjectConfig(dir string) (cfg *appDevProjectConfig, found bool, err error) {
	mp := filepath.Join(dir, sparkJSONRelPath)
	b, rerr := os.ReadFile(mp) //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard); path is cwd-relative.
	if rerr != nil {
		if os.IsNotExist(rerr) {
			return nil, false, nil
		}
		return nil, false, appsFileIOError(rerr, "read %s failed: %v", sparkJSONRelPath, rerr)
	}
	var doc sparkJSONDoc
	if jerr := json.Unmarshal(b, &doc); jerr != nil {
		return nil, true, appsFileIOError(jerr, "parse %s failed: %v", sparkJSONRelPath, jerr)
	}
	cfg = &appDevProjectConfig{
		Stack:          doc.Stack,
		Version:        doc.Version,
		BuildCommand:   doc.Build.Command,
		BuildOutput:    strings.TrimSpace(doc.Build.Output),
		BuildOutputCDN: strings.TrimSpace(doc.Build.OutputCDN),
		AppID:          strings.TrimSpace(doc.App.ID),
		AppURL:         strings.TrimSpace(doc.App.URL),
	}
	applyAppDevConfigDefaults(cfg)
	return cfg, true, nil
}

// applyAppDevConfigDefaults fills protocol defaults (§3): build.output →
// dist/output. build.command deliberately has no default — missing means
// buildless (§4), and build.output_cdn stays empty for Level 1.
func applyAppDevConfigDefaults(cfg *appDevProjectConfig) {
	if cfg.BuildOutput == "" {
		cfg.BuildOutput = appDevDefaultBuildOutput
	}
}

// Buildless reports whether the project declared no build command: packing
// uses the output directories as-is.
func (c *appDevProjectConfig) Buildless() bool { return len(c.BuildCommand) == 0 }

// writeSparkAppSection replaces the app state section of <dir>/spark.json
// with {id, url} after a successful publish (§3: the app section is owned by
// the deploy chain and replaced wholesale; declaration fields are never
// touched). Empty url omits the key. Creates the file if missing.
func writeSparkAppSection(dir, appID, appURL string) error {
	path := filepath.Join(dir, sparkJSONRelPath)
	doc := map[string]interface{}{}
	if b, err := os.ReadFile(path); err == nil { //nolint:forbidigo // see readAppDevProjectConfig.
		if jerr := json.Unmarshal(b, &doc); jerr != nil {
			return appsFileIOError(jerr, "parse %s failed: %v", sparkJSONRelPath, jerr)
		}
	} else if !os.IsNotExist(err) {
		return appsFileIOError(err, "read %s failed: %v", sparkJSONRelPath, err)
	}
	app := map[string]interface{}{"id": appID}
	if appURL != "" {
		app["url"] = appURL
	}
	doc["app"] = app
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return appsFileIOError(err, "marshal %s failed: %v", sparkJSONRelPath, err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil { //nolint:forbidigo // see above.
		return appsFileIOError(err, "write %s failed: %v", sparkJSONRelPath, err)
	}
	return nil
}

// writeSparkScaffoldFields merge-writes the scaffold-owned fields into
// <dir>/spark.json after template rendering: version is always stamped with
// the rendered package version (authoritative), stack is only filled when the
// template seed did not declare one, and every other field the seed shipped
// (dev/build declarations, unknown fields) is preserved (§3 字段所有权).
func writeSparkScaffoldFields(dir, stack, version string) error {
	path := filepath.Join(dir, sparkJSONRelPath)
	doc := map[string]interface{}{}
	if b, err := os.ReadFile(path); err == nil { //nolint:forbidigo // see readAppDevProjectConfig.
		if jerr := json.Unmarshal(b, &doc); jerr != nil {
			return appsFileIOError(jerr, "parse %s failed: %v", sparkJSONRelPath, jerr)
		}
	} else if !os.IsNotExist(err) {
		return appsFileIOError(err, "read %s failed: %v", sparkJSONRelPath, err)
	}
	if cur, _ := doc["stack"].(string); strings.TrimSpace(cur) == "" {
		doc["stack"] = stack
	}
	doc["version"] = version
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return appsFileIOError(err, "marshal %s failed: %v", sparkJSONRelPath, err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil { //nolint:forbidigo // see above.
		return appsFileIOError(err, "write %s failed: %v", sparkJSONRelPath, err)
	}
	return nil
}
