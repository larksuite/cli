// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
)

// appDevTemplatePkgPrefix is the npm package naming convention for artifact
// templates, aligned with miaoda-cli's TEMPLATE_PACKAGE_BY_STACK
// ("@lark-apaas/coding-template-" + stack short name).
const appDevTemplatePkgPrefix = "@lark-apaas/coding-template-"

// appDevTemplateEntryPrefix is the tarball path prefix that holds the
// renderable template files (npm tarballs root at "package/").
const appDevTemplateEntryPrefix = "package/template/"

// appDevRegistryBase is the npm registry used to resolve template packages.
// Package-level var so unit tests can point it at an httptest server.
var appDevRegistryBase = npmRegistry

// Decompression-bomb / runaway-template caps. Vars (not consts) so unit tests
// can shrink them to cover the rejection paths; defaults are far above any
// legitimate template.
var (
	appDevMaxTemplateTgzBytes     int64 = 20 * 1024 * 1024
	appDevMaxTemplateExtractBytes int64 = 100 * 1024 * 1024
	appDevMaxTemplateFiles              = 2000
)

// appDevTemplatePackageName maps a template short name to its npm package.
func appDevTemplatePackageName(template string) string {
	return appDevTemplatePkgPrefix + template
}

// npmPackageMeta is the subset of the npm registry package document the
// fetch needs: latest dist-tag plus each version's tarball URL.
type npmPackageMeta struct {
	DistTags map[string]string `json:"dist-tags"`
	Versions map[string]struct {
		Dist struct {
			Tarball string `json:"tarball"`
		} `json:"dist"`
	} `json:"versions"`
}

// fetchAppDevTemplateMeta resolves the template package's latest version and
// tarball URL from the npm registry. Only https tarball URLs are accepted.
func fetchAppDevTemplateMeta(ctx context.Context, pkg string) (version, tarballURL string, err error) {
	metaURL := strings.TrimRight(appDevRegistryBase, "/") + "/" + pkg
	body, err := appDevHTTPGet(ctx, metaURL, appDevMaxTemplateTgzBytes,
		"the template package may not be published yet; ask the artifact team, or check network/registry access")
	if err != nil {
		return "", "", err
	}
	var meta npmPackageMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return "", "", appsSubprocessEnvelopeError("npm registry metadata for %s is not valid JSON", pkg)
	}
	latest := meta.DistTags["latest"]
	if latest == "" {
		return "", "", appsSubprocessEnvelopeError("npm registry metadata for %s has no latest dist-tag", pkg)
	}
	v, ok := meta.Versions[latest]
	if !ok || v.Dist.Tarball == "" {
		return "", "", appsSubprocessEnvelopeError("npm registry metadata for %s@%s has no tarball URL", pkg, latest)
	}
	u, perr := url.Parse(v.Dist.Tarball)
	if perr != nil || u.Scheme != "https" {
		return "", "", appsSubprocessEnvelopeError("npm registry tarball URL for %s@%s is not https; refusing to download", pkg, latest)
	}
	// Same-origin constraint: npm registries serve tarballs from the registry
	// host itself, so a cross-host URL in the metadata is a red flag (metadata
	// tampering / registry compromise) — refuse rather than follow it.
	if reg, rerr := url.Parse(appDevRegistryBase); rerr != nil || u.Host != reg.Host {
		return "", "", appsSubprocessEnvelopeError("npm registry tarball URL host %q differs from registry host; refusing to download", u.Host)
	}
	return latest, v.Dist.Tarball, nil
}

// appDevHTTPGet fetches a URL with a hard size cap. notFoundHint decorates the
// 404 error (the caller knows what a missing resource means in its context).
func appDevHTTPGet(ctx context.Context, rawURL string, maxBytes int64, notFoundHint string) ([]byte, error) {
	//nolint:forbidigo // npm registry download is not a Lark API call; RuntimeContext.DoAPI does not apply.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "build registry request").WithCause(err)
	}
	resp, err := appDevNewTransferClient().Do(req) //nolint:forbidigo // see above.
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "npm registry request failed").WithCause(err).WithRetryable()
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"npm registry returned 404 for %s", rawURL).WithHint(notFoundHint)
	case resp.StatusCode >= 500:
		return nil, errs.NewNetworkError(errs.SubtypeNetworkServer,
			"npm registry returned HTTP %d", resp.StatusCode).WithRetryable()
	case resp.StatusCode >= 400:
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"npm registry returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "read registry response").WithCause(err).WithRetryable()
	}
	if int64(len(body)) > maxBytes {
		return nil, appsValidationError("registry response exceeds %d bytes limit", maxBytes).
			WithHint("the template package is unexpectedly large; contact the artifact team")
	}
	return body, nil
}

// renderedTemplate reports what renderAppDevTemplate materialized.
type renderedTemplate struct {
	ArchType interface{}
	Files    int
}

// templatePkgJSON is the subset of the template package's own package.json
// the renderer reads (archType rides on miaodaTemplate, set by the template).
type templatePkgJSON struct {
	MiaodaTemplate struct {
		ArchType interface{} `json:"archType"`
	} `json:"miaodaTemplate"`
}

// renamedTemplateFiles maps placeholder names shipped in the tarball to their
// real dotfile names (npm pack strips .npmrc; .gitignore conflicts with
// platform repos) — aligned with miaoda-cli's RENAME_FILES.
var renamedTemplateFiles = map[string]string{
	"_gitignore": ".gitignore",
	"_npmrc":     ".npmrc",
}

// placeholderTemplateFiles are the display-only files whose {{projectName}}
// placeholder is replaced after extraction — aligned with miaoda-cli's
// renderTemplate (package.json keeps a fixed name on purpose there).
var placeholderTemplateFiles = []string{"index.html", "README.md"}

// renderAppDevTemplate extracts the package/template/ subtree of an npm
// template tarball into targetDir and applies the rename + placeholder
// conventions. Only regular files under the template prefix are written;
// symlinks, hardlinks, and traversal paths are rejected or skipped.
func renderAppDevTemplate(targetDir, projectName string, tgz []byte) (*renderedTemplate, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, appsSubprocessEnvelopeError("template tarball is not gzip: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var total int64
	var pkgJSONRaw []byte
	files := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, appsSubprocessEnvelopeError("read template tarball: %v", err)
		}
		raw := strings.TrimPrefix(hdr.Name, "./")
		// Fail closed on the RAW entry name before any cleaning: a template
		// carrying traversal or backslash entries is malformed or malicious,
		// and partially rendering it would hide that.
		if isUnsafeRelPath(raw) || strings.ContainsRune(raw, '\\') {
			return nil, appsSubprocessEnvelopeError("template tarball entry %q escapes the target directory; refusing to extract", hdr.Name)
		}
		name := path.Clean(raw)
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			// Never materialize links from a downloaded archive — a link
			// pointing outside targetDir would bypass the path checks below.
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if name == "package/package.json" {
			raw, err := io.ReadAll(io.LimitReader(tr, 1<<20))
			if err != nil {
				return nil, appsSubprocessEnvelopeError("read template package.json: %v", err)
			}
			pkgJSONRaw = raw
			continue
		}
		if !strings.HasPrefix(name, appDevTemplateEntryPrefix) {
			continue
		}
		rel := strings.TrimPrefix(name, appDevTemplateEntryPrefix)
		// isUnsafeRelPath handles forward-slash traversal; the extra checks
		// reject backslashes and Windows drive/reserved forms that only bite
		// after filepath.FromSlash on Windows (security-review requirement).
		if rel == "" || isUnsafeRelPath(rel) ||
			strings.ContainsRune(rel, '\\') || !filepath.IsLocal(filepath.FromSlash(rel)) {
			return nil, appsSubprocessEnvelopeError("template tarball entry %q escapes the target directory; refusing to extract", hdr.Name)
		}
		files++
		if files > appDevMaxTemplateFiles {
			return nil, appsValidationError("template contains more than %d files; refusing to extract", appDevMaxTemplateFiles).
				WithHint("the template package looks malformed; contact the artifact team")
		}
		dest := filepath.Join(targetDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil { //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard); targetDir is validated relative-only.
			return nil, appsFileIOError(err, "create template directory for %s failed: %v", rel, err)
		}
		remaining := appDevMaxTemplateExtractBytes - total
		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:forbidigo // see above.
		if err != nil {
			return nil, appsFileIOError(err, "create template file %s failed: %v", rel, err)
		}
		n, err := io.Copy(out, io.LimitReader(tr, remaining+1))
		out.Close()
		if err != nil {
			return nil, appsFileIOError(err, "write template file %s failed: %v", rel, err)
		}
		total += n
		if total > appDevMaxTemplateExtractBytes {
			return nil, appsValidationError("template extraction exceeds %d bytes limit", appDevMaxTemplateExtractBytes).
				WithHint("the template package looks malformed; contact the artifact team")
		}
	}

	for from, to := range renamedTemplateFiles {
		fromPath := filepath.Join(targetDir, from)
		if _, err := os.Stat(fromPath); err == nil { //nolint:forbidigo // see above.
			if err := os.Rename(fromPath, filepath.Join(targetDir, to)); err != nil { //nolint:forbidigo // see above.
				return nil, appsFileIOError(err, "rename template file %s failed: %v", from, err)
			}
		}
	}
	for _, rel := range placeholderTemplateFiles {
		p := filepath.Join(targetDir, rel)
		b, err := os.ReadFile(p) //nolint:forbidigo // see above.
		if err != nil {
			continue
		}
		replaced := strings.ReplaceAll(string(b), "{{projectName}}", projectName)
		if replaced != string(b) {
			if err := os.WriteFile(p, []byte(replaced), 0o644); err != nil { //nolint:forbidigo // see above.
				return nil, appsFileIOError(err, "write template file %s failed: %v", rel, err)
			}
		}
	}

	rendered := &renderedTemplate{Files: files}
	if len(pkgJSONRaw) > 0 {
		var pkg templatePkgJSON
		if err := json.Unmarshal(pkgJSONRaw, &pkg); err == nil {
			rendered.ArchType = pkg.MiaodaTemplate.ArchType
		}
	}
	return rendered, nil
}

// writeAppDevSparkMeta merge-writes {stack, version, archType} into
// <dir>/.spark/meta.json, creating the directory as needed. Field names align
// with miaoda-cli's SparkMeta so downstream tooling reads one format.
func writeAppDevSparkMeta(dir, stack, version string, archType interface{}) error {
	sparkDir := filepath.Join(dir, ".spark")
	if err := os.MkdirAll(sparkDir, 0o755); err != nil { //nolint:forbidigo // see renderAppDevTemplate.
		return appsFileIOError(err, "create .spark directory failed: %v", err)
	}
	metaPath := filepath.Join(dir, metaRelPath)
	meta := map[string]interface{}{}
	if b, err := os.ReadFile(metaPath); err == nil { //nolint:forbidigo // see above.
		_ = json.Unmarshal(b, &meta)
	}
	meta["stack"] = stack
	meta["version"] = version
	if archType != nil {
		meta["archType"] = archType
	}
	out, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return appsFileIOError(err, "marshal %s failed: %v", metaRelPath, err)
	}
	if err := os.WriteFile(metaPath, append(out, '\n'), 0o644); err != nil { //nolint:forbidigo // see above.
		return appsFileIOError(err, "write %s failed: %v", metaRelPath, err)
	}
	return nil
}
