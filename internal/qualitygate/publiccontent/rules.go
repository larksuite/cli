// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package publiccontent

import (
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/larksuite/cli/internal/qualitygate/report"
)

var (
	credentialAssignmentRE = regexp.MustCompile(`(?i)["']?\b[A-Za-z0-9_-]*(?:api[_-]?key|access[_-]?key|private[_-]?key|secret|password|passwd|token|webhook|access[_-]?token|client[_-]?secret)[A-Za-z0-9_-]*\b["']?\s*[:=]\s*(?:"((?:\\.|[^"\\])*)"|'((?:\\.|[^'\\])*)'|(\$\([^)]*\))|(\$\{\{[^}]+\}\})|([^"'\s,}\]]+))`)
	jwtLikeRE              = regexp.MustCompile(`\b[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)
	credentialURLRE        = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^/\s:@]*:[^@\s/]+@[^)\s]+`)
	bearerHeaderRE         = regexp.MustCompile(`(?i)(?:\bAuthorization\s*:\s*Bearer\s+|["']Authorization["']\s*:\s*["']Bearer\s+)[A-Za-z0-9._+/=-]{12,}`)
	semanticBearerHeaderRE = regexp.MustCompile(`(?i)(?:\bAuthorization\s*:\s*Bearer\s+[^"'\s,}\]]+|["']Authorization["']\s*:\s*["']Bearer\s+[^"'\\\s,}\]]+)`)
	changeIDTrailerRE      = regexp.MustCompile(`(?i)^\s*Change-Id:\s*\S+`)
	reviewedOnTrailerRE    = regexp.MustCompile(`(?i)^\s*Reviewed-on:\s*\S+`)
	ccmHarnessTrailerRE    = regexp.MustCompile(`(?i)\bCCM-Harness:\s*\S+`)
	privateIPv4RE          = regexp.MustCompile(`\b(?:10\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}|192\.168\.[0-9]{1,3}\.[0-9]{1,3}|172\.(?:1[6-9]|2[0-9]|3[0-1])\.[0-9]{1,3}\.[0-9]{1,3})\b`)
	automationBranchRE     = regexp.MustCompile(`(?i)(^|/)(bot|automation)[-/]`)
)

func actionForRule(rule string) report.Action {
	switch rule {
	case "public_content_generic_credential",
		"public_content_private_key_block",
		"public_content_jwt_like_token",
		"public_content_bearer_header",
		"public_content_credential_url",
		"public_content_change_id_trailer",
		"public_content_reviewed_on_trailer",
		"public_content_provenance_marker",
		"public_content_detector_fingerprint",
		"public_content_harness_metadata",
		"public_content_ccm_harness_trailer":
		return report.ActionReject
	case "public_content_private_ipv4",
		"public_content_automation_branch":
		return report.ActionWarning
	default:
		return report.ActionWarning
	}
}

func isPlaceholderValue(value string) bool {
	trimmed := strings.Trim(value, `"'`)
	normalized := strings.ToLower(trimmed)
	if normalized == "" ||
		normalized == "=" ||
		percentWrappedPlaceholder(normalized) ||
		angleWrappedPlaceholder(normalized) ||
		urlWithAnglePlaceholder(normalized) ||
		isCredentialReferenceValue(trimmed) {
		return true
	}
	return namedPlaceholderValue(normalized)
}

func namedPlaceholderValue(value string) bool {
	switch value {
	case "...", "placeholder", "redacted", "<redacted>", "xxxx", "test-secret":
		return true
	}
	return strings.Contains(value, "cli_example") || allXPlaceholder(value)
}

func allXPlaceholder(value string) bool {
	if len(value) < 4 {
		return false
	}
	for _, r := range value {
		if r != 'x' {
			return false
		}
	}
	return true
}

func urlWithAnglePlaceholder(value string) bool {
	if !strings.Contains(value, "://") ||
		!strings.Contains(value, "<") ||
		!strings.Contains(value, ">") {
		return false
	}
	return !urlRemainderLooksCredentialLike(removeAnglePlaceholders(value))
}

func removeAnglePlaceholders(value string) string {
	var out strings.Builder
	for len(value) > 0 {
		start := strings.Index(value, "<")
		if start < 0 {
			out.WriteString(value)
			break
		}
		out.WriteString(value[:start])
		end := strings.Index(value[start+1:], ">")
		if end < 0 {
			out.WriteString(value[start:])
			break
		}
		value = value[start+end+2:]
	}
	return out.String()
}

func urlRemainderLooksCredentialLike(value string) bool {
	normalized := strings.ToLower(value)
	for _, marker := range []string{
		"secret",
		"token",
		"password",
		"passwd",
		"api_key",
		"apikey",
		"private_key",
		"privatekey",
		"client_secret",
		"clientsecret",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	for _, part := range strings.FieldsFunc(normalized, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-')
	}) {
		if credentialShapedIdentifier(part) || longCredentialSegment(part) {
			return true
		}
	}
	return false
}

func longCredentialSegment(value string) bool {
	if len(value) < 16 {
		return false
	}
	var hasLetter, hasDigit bool
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return hasLetter || hasDigit
}

func isCredentialReferenceValue(value string) bool {
	normalized := strings.ToLower(value)
	switch {
	case strings.HasPrefix(normalized, "${{"):
		return githubExpressionReference(normalized)
	case strings.HasPrefix(normalized, "$("):
		return !commandSubstitutionLooksCredentialLike(normalized)
	case strings.HasPrefix(normalized, "process.env."):
		return credentialReferenceIdentifier(strings.TrimPrefix(normalized, "process.env."))
	case strings.HasPrefix(normalized, "${"):
		return credentialReferenceIdentifier(strings.TrimSuffix(strings.TrimPrefix(normalized, "${"), "}"))
	case strings.HasPrefix(value, "$"):
		return credentialReferenceIdentifier(strings.TrimPrefix(normalized, "$"))
	default:
		return false
	}
}

func commandSubstitutionLooksCredentialLike(value string) bool {
	if !strings.HasPrefix(value, "$(") || !strings.HasSuffix(value, ")") {
		return false
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(value, "$("), ")")
	for _, part := range strings.FieldsFunc(inner, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-')
	}) {
		if credentialShapedIdentifier(part) || longCredentialSegment(part) {
			return true
		}
	}
	return false
}

func githubExpressionReference(value string) bool {
	if !strings.HasPrefix(value, "${{") || !strings.HasSuffix(value, "}}") {
		return false
	}
	expr := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "${{"), "}}"))
	switch {
	case strings.HasPrefix(expr, "secrets."):
		return dottedReferenceIdentifier(strings.TrimPrefix(expr, "secrets."))
	case strings.HasPrefix(expr, "env."):
		return dottedReferenceIdentifier(strings.TrimPrefix(expr, "env."))
	case strings.HasPrefix(expr, "vars."):
		return dottedReferenceIdentifier(strings.TrimPrefix(expr, "vars."))
	case expr == "github.token":
		return true
	default:
		return false
	}
}

func dottedReferenceIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, part := range strings.Split(value, ".") {
		if !referenceIdentifier(part) {
			return false
		}
	}
	return true
}

func credentialReferenceIdentifier(value string) bool {
	return referenceIdentifier(value) && !credentialShapedIdentifier(value)
}

func referenceIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9' && i > 0:
		case r == '_' && i > 0:
		default:
			return false
		}
	}
	return true
}

func angleWrappedPlaceholder(value string) bool {
	if len(value) < 3 || !strings.HasPrefix(value, "<") || !strings.HasSuffix(value, ">") {
		return false
	}
	return anglePlaceholderIdentifier(strings.Trim(value, "<>"))
}

func percentWrappedPlaceholder(value string) bool {
	if len(value) < 3 || !strings.HasPrefix(value, "%") || !strings.HasSuffix(value, "%") {
		return false
	}
	inner := strings.Trim(value, "%")
	return delimitedPlaceholderIdentifier(inner) && !credentialShapedIdentifier(inner)
}

func delimitedPlaceholderIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func anglePlaceholderIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	if credentialShapedIdentifier(value) {
		return false
	}
	switch value {
	case "token",
		"id",
		"userid",
		"openid",
		"key",
		"secret",
		"password",
		"api-key",
		"user-id",
		"open-id",
		"client-secret",
		"access-token",
		"refresh-token",
		"auth-token",
		"bearer-token",
		"session-token",
		"service-token":
		return true
	}
	for _, suffix := range []string{"_token", "_id", "_key", "_secret", "_password"} {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	for _, suffix := range []string{"-token", "-id", "-key", "-secret", "-password"} {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return false
}

func credentialShapedValue(value string) bool {
	normalized := strings.ToLower(strings.Trim(value, `"'<>`))
	return credentialShapedIdentifier(normalized)
}

func credentialShapedIdentifier(value string) bool {
	switch {
	case strings.HasPrefix(value, "sk_live_"),
		strings.HasPrefix(value, "sk_test_"),
		strings.HasPrefix(value, "ghp_"),
		strings.HasPrefix(value, "gho_"),
		strings.HasPrefix(value, "ghu_"),
		strings.HasPrefix(value, "github_pat_"),
		strings.HasPrefix(value, "xoxb_"),
		strings.HasPrefix(value, "xoxp_"),
		strings.HasPrefix(value, "xoxa_"):
		return true
	case strings.HasPrefix(value, "real-") &&
		(strings.Contains(value, "secret") ||
			strings.Contains(value, "token") ||
			strings.Contains(value, "key") ||
			strings.Contains(value, "password")):
		return true
	default:
		return false
	}
}

func resourceTokenPlaceholderValue(value string) bool {
	normalized := strings.ToLower(strings.Trim(value, `"'`))
	switch normalized {
	case "wiki_token",
		"folder_token",
		"obj_token",
		"spreadsheet_token",
		"file_token",
		"doc_token",
		"node_token",
		"parent_node_token",
		"origin_node_token",
		"drive_route_token":
		return true
	default:
		return minuteTokenFixturePlaceholder(normalized)
	}
}

func minuteTokenFixturePlaceholder(value string) bool {
	if value == "minute_no_meta" {
		return true
	}
	suffix, ok := strings.CutPrefix(value, "minute_")
	if !ok || suffix == "" {
		return false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func provenanceMarker(line string) bool {
	normalized := strings.ToLower(line)
	markers := []string{
		"generat" + "ed by tool",
		"creat" + "ed by tool",
		"generat" + "ed by automation",
		"creat" + "ed by automation",
		"machine-" + "generated",
		"generated with automated",
		"generated with automation",
		"🤖 generated",
	}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	if strings.HasPrefix(normalized, "co-authored-by:") &&
		(strings.Contains(normalized, "<bot@") ||
			strings.Contains(normalized, " bot@") ||
			strings.Contains(normalized, "[bot]") ||
			strings.Contains(normalized, "automation") ||
			strings.Contains(normalized, "automated-code-assistant")) {
		return true
	}
	return false
}

// Detector fingerprint checks are intentionally scoped to public rule/config
// files. They do not try to hide this package's implementation; they prevent
// publishing reusable detector identifiers in external-facing rule bundles.
func isDetectorRuleFile(path string) bool {
	normalized := filepath.ToSlash(path)
	base := filepath.Base(normalized)
	return base == ".gitleaks.toml" ||
		strings.Contains(normalized, "public-rules/") ||
		strings.Contains(normalized, "public_rules/")
}

func detectorFingerprint(line string) bool {
	normalized := strings.ToLower(line)
	fingerprints := []string{
		strings.Join([]string{"public", "content", "leakage"}, "-"),
		strings.Join([]string{"public", "content", "detector"}, "-"),
		"publiccontent",
	}
	for _, fingerprint := range fingerprints {
		if strings.Contains(normalized, fingerprint) {
			return true
		}
	}
	return false
}

func redactCredentialURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return "<credential-url>"
	}
	u.User = url.UserPassword("<user>", "<redacted>")
	return u.String()
}
