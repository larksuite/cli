// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build scopeexport

package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts"
	"github.com/spf13/cobra"
)

// registerExportScopes adds the hidden export-scopes command in scopeexport builds.
func registerExportScopes(cmd *cobra.Command, f *cmdutil.Factory) {
	cmd.AddCommand(newCmdAuthExportScopes(f))
}

func newCmdAuthExportScopes(f *cmdutil.Factory) *cobra.Command {
	var (
		brand      string
		version    string
		outputPath string
	)
	cmd := &cobra.Command{
		Use:    "export-scopes",
		Short:  "Export brand scopes JSON (build tool, hidden)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			effectiveVersion := version
			if effectiveVersion == "" {
				effectiveVersion = packageJSONVersion()
			}
			if effectiveVersion == "" {
				effectiveVersion = build.Version
			}
			if effectiveVersion == "" || effectiveVersion == "DEV" {
				fmt.Fprintln(cmd.ErrOrStderr(),
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
				fmt.Fprint(cmd.OutOrStdout(), string(data))
				return nil
			}
			safe, err := validate.SafeOutputPath(outputPath)
			if err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --output path: %v", err).WithParam("--output").WithCause(err)
			}
			if err := vfs.WriteFile(safe, data, 0644); err != nil {
				return errs.NewInternalError(errs.SubtypeStorage, "failed to write %s: %v", safe, err).WithCause(err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&brand, "brand", "", "target brand: feishu | lark (required)")
	cmd.Flags().StringVar(&version, "version", "", "version string written to output (default: build.Version)")
	cmd.Flags().StringVar(&outputPath, "output", "", "write JSON to this file (default: stdout)")
	_ = cmd.MarkFlagRequired("brand")
	return cmd
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
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --brand %q: must be feishu or lark", brandStr).WithParam("--brand")
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
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "open embedded catalog snapshot: %v", err).WithCause(err)
	}
	catalog := snapshot.Catalog()
	if err := catalog.Preload(catalog.Names()...); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "preload embedded catalog: %v", err).WithCause(err)
	}
	if len(catalog.Names()) == 0 {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "meta registry is empty for brand=%s", brandStr)
	}
	resolver := newDomainResolver(catalog, shortcuts.AllShortcuts())
	doc := &brandScopesDoc{Version: version, Scopes: map[string]domainScopes{}}
	for _, d := range resolver.sorted(brand) {
		user := normalizeScopes(filterBatchExcludedScopes(resolver.scopesFor([]string{d}, "user", brand)))
		tenant := normalizeScopes(filterBatchExcludedScopes(resolver.scopesFor([]string{d}, "bot", brand)))
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
// absent or unparseable (e.g. `go test` runs from the package directory),
// leaving the caller to fall back to build.Version.
func packageJSONVersion() string {
	data, err := os.ReadFile("package.json")
	if err != nil {
		return ""
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
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
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "failed to marshal scopes: %v", err).WithCause(err)
	}
	return buf.Bytes(), nil // Encode already appends a trailing '\n'
}
