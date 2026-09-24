// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build scopeexport

// Command scopes-export writes the brand scopes JSON consumed by the downstream
// scopes publisher. It is a build-only tool gated behind the scopeexport tag and
// is never compiled into the released CLI binary. It reads only the embedded API
// catalog and needs no credentials or network.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/larksuite/cli/internal/apiscopes"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts"
)

func main() {
	brand := flag.String("brand", "", "target brand: feishu | lark (required)")
	version := flag.String("version", "", "version string written to output (default: package.json version, else build.Version)")
	outputPath := flag.String("output", "", "write JSON to this file (default: stdout)")
	flag.Parse()

	if err := run(*brand, *version, *outputPath); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(brand, version, outputPath string) error {
	effectiveVersion := version
	if effectiveVersion == "" {
		effectiveVersion = packageJSONVersion()
	}
	if effectiveVersion == "" {
		effectiveVersion = build.Version
	}
	if effectiveVersion == "" || effectiveVersion == "DEV" {
		fmt.Fprintln(os.Stderr,
			`warning: exporting with version "DEV"; pass --version to set the published version`)
	}

	doc, err := buildBrandScopesDoc(brand, effectiveVersion)
	if err != nil {
		return err
	}
	data, err := marshalBrandScopesDoc(doc)
	if err != nil {
		return err
	}

	if outputPath == "" {
		if _, err := os.Stdout.Write(data); err != nil {
			return fmt.Errorf("failed to write scopes JSON to stdout: %w", err)
		}
		return nil
	}
	safe, err := validate.SafeOutputPath(outputPath)
	if err != nil {
		return fmt.Errorf("invalid --output path: %w", err)
	}
	if err := vfs.WriteFile(safe, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", safe, err)
	}
	return nil
}

type i18nText struct {
	ZhCn string `json:"zh_cn"`
	EnUs string `json:"en_us"`
}

type domainScopes struct {
	I18nName     i18nText `json:"i18n_name"`
	I18nDesc     i18nText `json:"i18n_desc"`
	TenantScopes []string `json:"tenant_scopes"`
	UserScopes   []string `json:"user_scopes"`
}

type brandScopesDoc struct {
	Version string                  `json:"version"`
	Scopes  map[string]domainScopes `json:"scopes"`
}

// parseBrandExact enforces an exact feishu|lark match. core.ParseBrand must NOT
// be used here: it silently coerces any non-"lark" value to feishu, which would
// emit a wrong-brand file for a typo'd --brand.
func parseBrandExact(brandStr string) (core.LarkBrand, error) {
	switch brandStr {
	case "feishu":
		return core.BrandFeishu, nil
	case "lark":
		return core.BrandLark, nil
	default:
		return "", fmt.Errorf("invalid --brand %q: must be feishu or lark", brandStr)
	}
}

// buildBrandScopesDoc resolves the final brand scopes document from the embedded
// API catalog. The catalog carries no remote overlay, so the result is a pure
// function of the compiled-in meta — deterministic across runs, which the
// published scopes JSON supply chain requires. Brand differentiation flows
// through the resolver's brand parameter (which reaches
// shortcuts.IsShortcutServiceAvailable(service, brand)), so resolving both
// brands in one process is correct.
func buildBrandScopesDoc(brandStr, version string) (*brandScopesDoc, error) {
	brand, err := parseBrandExact(brandStr)
	if err != nil {
		return nil, err
	}
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		return nil, fmt.Errorf("open embedded catalog snapshot: %w", err)
	}
	catalog := snapshot.Catalog()
	if err := catalog.Preload(catalog.Names()...); err != nil {
		return nil, fmt.Errorf("preload embedded catalog: %w", err)
	}
	if len(catalog.Names()) == 0 {
		return nil, fmt.Errorf("meta registry is empty for brand=%s", brandStr)
	}
	resolver := apiscopes.NewResolver(catalog, shortcuts.AllShortcuts())
	doc := &brandScopesDoc{Version: version, Scopes: map[string]domainScopes{}}
	for _, d := range resolver.Sorted(brand) {
		user := normalizeScopes(apiscopes.FilterBatchExcludedScopes(resolver.ScopesFor([]string{d}, "user", brand)))
		tenant := normalizeScopes(apiscopes.FilterBatchExcludedScopes(resolver.ScopesFor([]string{d}, "bot", brand)))
		if len(user) == 0 && len(tenant) == 0 {
			continue // skip empty domains
		}
		doc.Scopes[d] = domainScopes{
			I18nName:     i18nText{ZhCn: titleOrDomain(d, "zh"), EnUs: titleOrDomain(d, "en")},
			I18nDesc:     i18nText{ZhCn: registry.GetServiceDescription(d, "zh"), EnUs: registry.GetServiceDescription(d, "en")},
			TenantScopes: tenant,
			UserScopes:   user,
		}
	}
	return doc, nil
}

// normalizeScopes returns a non-nil, de-duplicated, sorted slice so empty lists
// marshal as [] (not null) and ordering is deterministic.
func normalizeScopes(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// titleOrDomain returns the localized service title, falling back to the domain
// name itself when the service is not in service_descriptions.
func titleOrDomain(d, lang string) string {
	if t := registry.GetServiceTitle(d, lang); t != "" {
		return t
	}
	return d
}

// packageJSONVersion reads the version field from ./package.json — the CLI
// checkout's npm manifest — so the exported document carries the same version
// the released binary ships under, matching the downstream scopes publisher
// that stamped its version from package.json. It returns "" when the file is
// absent, unparseable, or not the CLI's own manifest (name != @larksuite/cli),
// leaving the caller to fall back to build.Version. The name guard stops an
// unrelated package.json in the working directory from stamping a foreign
// version.
func packageJSONVersion() string {
	data, err := os.ReadFile("package.json")
	if err != nil {
		return ""
	}
	var pkg struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	if pkg.Name != "@larksuite/cli" {
		return ""
	}
	return pkg.Version
}

// marshalBrandScopesDoc serializes with 2-space indent + trailing newline to
// match the downstream publisher's `JSON.stringify(result, null, 2)+"\n"`
// encoding. HTML escaping is disabled: JSON.stringify never escapes `&`/`<`/`>`,
// but json.Marshal/MarshalIndent do by default — leaving it on would corrupt any
// service description containing those characters (e.g. "role & permission
// management") and break the byte-for-byte drop-in contract.
func marshalBrandScopesDoc(doc *brandScopesDoc) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("failed to marshal scopes: %w", err)
	}
	return buf.Bytes(), nil // Encode already appends a trailing '\n'
}
