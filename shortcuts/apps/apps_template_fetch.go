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
	"sort"
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

// appDevRegistries are the npm registries used to resolve template packages,
// tried in order: npmmirror first (fast inside CN), the official registry as
// fallback — a freshly published package may not have synced to the mirror
// yet, and mirror outages must not block scaffolding. Package-level var so
// unit tests can point it at httptest servers.
var appDevRegistries = []string{npmRegistry, "https://registry.npmjs.org"}

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

// fetchAppDevTemplate resolves and downloads the template package, trying
// each registry in registries until one succeeds (nil/empty = the built-in
// appDevRegistries fallback chain; an explicit --registry passes a single
// entry, so a failure is deterministic instead of silently shifting to
// another source). requested pins a specific version or dist-tag ("" =
// latest). onFallback is called with a human-readable note before each retry
// (nil to skip).
func fetchAppDevTemplate(ctx context.Context, pkg, requested string, registries []string, onFallback func(note string)) (version string, tgz []byte, err error) {
	if len(registries) == 0 {
		registries = appDevRegistries
	}
	var lastErr error
	for i, base := range registries {
		if i > 0 && onFallback != nil {
			onFallback(strings.TrimRight(registries[i-1], "/") + " failed, falling back to " + strings.TrimRight(base, "/"))
		}
		v, tarballURL, err := fetchAppDevTemplateMeta(ctx, base, pkg, requested)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := appDevHTTPGet(ctx, tarballURL, appDevMaxTemplateTgzBytes,
			"the template tarball is missing on the registry; contact the artifact team")
		if err != nil {
			lastErr = err
			continue
		}
		return v, body, nil
	}
	if p, ok := errs.ProblemOf(lastErr); ok && strings.TrimSpace(p.Hint) == "" {
		p.Hint = "all registries failed (" + strings.Join(registries, ", ") + "); check network access and whether the template package is published"
	}
	return "", nil, lastErr
}

// fetchAppDevTemplateMeta resolves the template package's version and
// tarball URL from one npm registry. requested may be a dist-tag (checked
// first) or an exact version; "" means the latest dist-tag. Only https
// tarball URLs on the same registry host are accepted.
func fetchAppDevTemplateMeta(ctx context.Context, registryBase, pkg, requested string) (version, tarballURL string, err error) {
	metaURL := strings.TrimRight(registryBase, "/") + "/" + pkg
	body, err := appDevHTTPGet(ctx, metaURL, appDevMaxTemplateTgzBytes,
		"the template package may not be published yet; ask the artifact team, or check network/registry access")
	if err != nil {
		return "", "", err
	}
	var meta npmPackageMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return "", "", appsSubprocessEnvelopeError("npm registry metadata for %s is not valid JSON", pkg)
	}
	resolved := strings.TrimSpace(requested)
	if resolved == "" {
		resolved = "latest"
	}
	// A dist-tag wins over a literal version of the same name (mirrors npm).
	if tagged := meta.DistTags[resolved]; tagged != "" {
		resolved = tagged
	}
	v, ok := meta.Versions[resolved]
	if !ok {
		tags := make([]string, 0, len(meta.DistTags))
		for t := range meta.DistTags {
			tags = append(tags, t)
		}
		sort.Strings(tags)
		return "", "", errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"npm registry has no version or dist-tag %q for %s", requested, pkg).
			WithHint("pass an exact published version or a dist-tag with --template-version; available dist-tags: " + strings.Join(tags, ", "))
	}
	if v.Dist.Tarball == "" {
		return "", "", appsSubprocessEnvelopeError("npm registry metadata for %s@%s has no tarball URL", pkg, resolved)
	}
	u, perr := url.Parse(v.Dist.Tarball)
	if perr != nil || u.Scheme != "https" {
		return "", "", appsSubprocessEnvelopeError("npm registry tarball URL for %s@%s is not https; refusing to download", pkg, resolved)
	}
	// Same-origin constraint: npm registries serve tarballs from the registry
	// host itself, so a cross-host URL in the metadata is a red flag (metadata
	// tampering / registry compromise) — refuse rather than follow it.
	if reg, rerr := url.Parse(registryBase); rerr != nil || u.Host != reg.Host {
		return "", "", appsSubprocessEnvelopeError("npm registry tarball URL host %q differs from registry host; refusing to download", u.Host)
	}
	return resolved, v.Dist.Tarball, nil
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
	Files int
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
	// Count EVERY decompressed byte (headers, skipped entries, extracted
	// data) so a gzip bomb hiding in entries the walk skips still trips the
	// cap — the tar reader "skips" by reading through this counter.
	counted := &countingReader{r: gz}
	tr := tar.NewReader(counted)
	files := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, appsSubprocessEnvelopeError("read template tarball: %v", err)
		}
		if counted.n > appDevMaxTemplateExtractBytes {
			return nil, appsValidationError("template extraction exceeds %d bytes limit", appDevMaxTemplateExtractBytes).
				WithHint("the template package looks malformed; contact the artifact team")
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
		remaining := appDevMaxTemplateExtractBytes - counted.n
		if remaining < 0 {
			remaining = 0
		}
		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:forbidigo // see above.
		if err != nil {
			return nil, appsFileIOError(err, "create template file %s failed: %v", rel, err)
		}
		_, err = io.Copy(out, io.LimitReader(tr, remaining+1))
		out.Close()
		if err != nil {
			return nil, appsFileIOError(err, "write template file %s failed: %v", rel, err)
		}
		if counted.n > appDevMaxTemplateExtractBytes {
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

	return &renderedTemplate{Files: files}, nil
}

// countingReader counts bytes read through it (decompressed tar stream).
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
