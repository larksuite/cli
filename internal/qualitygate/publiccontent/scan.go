// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package publiccontent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	privateKeyBeginPrefix = "-----" + "BEGIN "
	privateKeyEndPrefix   = "-----" + "END "
	privateKeyMarker      = "PRIVATE " + "KEY-----"
)

func ScanFile(path string, data []byte) []Finding {
	return scanText(filepath.ToSlash(path), "file", string(data), isDetectorRuleFile(path))
}

func semanticCandidate(file, source, text string, line int) []Finding {
	excerpt := redactedSemanticExcerpt(text)
	if excerpt == "" {
		return nil
	}
	return []Finding{newFinding("public_content_semantic_candidate", file, line, source, excerpt)}
}

func scanText(file, source, text string, detectorFile bool) []Finding {
	var out []Finding
	lines := strings.Split(text, "\n")
	inPrivateKey := false
	privateKeyLine := 0
	for i, line := range lines {
		lineNo := i + 1
		if strings.Contains(line, privateKeyBeginPrefix) && strings.Contains(line, privateKeyMarker) {
			inPrivateKey = true
			privateKeyLine = lineNo
		}
		if inPrivateKey && strings.Contains(line, privateKeyEndPrefix) && strings.Contains(line, privateKeyMarker) {
			out = append(out, newFinding("public_content_private_key_block", file, privateKeyLine, source, "private key block"))
			inPrivateKey = false
		}
		for _, match := range credentialAssignmentRE.FindAllStringSubmatch(line, -1) {
			if !isCredentialAssignmentMatch(match[0]) {
				continue
			}
			value := credentialAssignmentValue(match)
			keyName, _ := normalizedCredentialAssignmentKey(match[0])
			if value == "" ||
				isNonSecretLiteralValue(value) ||
				isBenignCodeCredentialExpression(file, line, match[0], value) ||
				isPlaceholderValue(value) ||
				isPermissionScopeIdentifierAssignment(keyName, value) ||
				isResourceTokenPlaceholderAssignment(keyName, value) {
				continue
			}
			if looksLikeEqualityComparison(value) {
				continue
			}
			out = append(out, newFinding("public_content_generic_credential", file, lineNo, source, redactAssignment(match[0])))
		}
		for _, match := range jwtLikeRE.FindAllString(line, -1) {
			if !isJWTToken(match) {
				continue
			}
			out = append(out, newFinding("public_content_jwt_like_token", file, lineNo, source, redactToken(match)))
		}
		for _, match := range bearerHeaderRE.FindAllString(line, -1) {
			if isPlaceholderBearerHeader(match) {
				continue
			}
			out = append(out, newFinding("public_content_bearer_header", file, lineNo, source, "Authorization: Bearer <redacted>"))
		}
		for _, match := range credentialURLRE.FindAllString(line, -1) {
			if isPlaceholderCredentialURL(file, match) {
				continue
			}
			out = append(out, newFinding("public_content_credential_url", file, lineNo, source, redactCredentialURL(match)))
		}
		for _, match := range privateIPv4RE.FindAllString(line, -1) {
			if !warnForPrivateIPv4(file) {
				continue
			}
			out = append(out, newFinding("public_content_private_ipv4", file, lineNo, source, match))
		}
		if source == "branch" && automationBranchRE.MatchString(line) {
			out = append(out, newFinding("public_content_automation_branch", file, lineNo, source, "automation branch marker"))
		}
		switch {
		case changeIDTrailerRE.MatchString(line):
			out = append(out, newFinding("public_content_change_id_trailer", file, lineNo, source, "Change-Id: <redacted>"))
		case reviewedOnTrailerRE.MatchString(line):
			out = append(out, newFinding("public_content_reviewed_on_trailer", file, lineNo, source, "Reviewed-on: <redacted>"))
		case ccmHarnessTrailerRE.MatchString(line):
			out = append(out, newFinding("public_content_ccm_harness_trailer", file, lineNo, source, "CCM-Harness: <redacted>"))
		}
		if provenanceMarker(line) {
			out = append(out, newFinding("public_content_provenance_marker", file, lineNo, source, "provenance marker"))
		}
		if strings.Contains(line, "/tmp/harness-agent") {
			out = append(out, newFinding("public_content_harness_metadata", file, lineNo, source, "/tmp/harness-agent"))
		}
		if detectorFile && detectorFingerprint(line) {
			out = append(out, newFinding("public_content_detector_fingerprint", file, lineNo, source, "public detector fingerprint"))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Rule < out[j].Rule
	})
	return out
}

func isCredentialAssignmentMatch(match string) bool {
	name, value, ok := normalizedCredentialAssignment(match)
	if !ok {
		return false
	}
	if isWebhookCredentialKey(name) && webhookAssignmentValueLooksCredentialLike(value) {
		return true
	}
	if isBenignTokenField(name) && !credentialShapedValue(value) {
		return false
	}
	if isWeakTokenCredentialKey(name) && !weakTokenValueLooksCredentialLike(value) {
		return false
	}
	return isExplicitCredentialKey(name)
}

func normalizedCredentialAssignmentKey(match string) (string, bool) {
	key, _, ok := normalizedCredentialAssignment(match)
	return key, ok
}

func normalizedCredentialAssignment(match string) (string, string, bool) {
	key, ok := credentialAssignmentKey(match)
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	submatches := credentialAssignmentRE.FindStringSubmatch(match)
	return normalizedCredentialKey(strings.Trim(key, `"'`)), credentialAssignmentValue(submatches), true
}

func normalizedCredentialKey(key string) string {
	key = strings.TrimSpace(key)
	var out []rune
	var prev rune
	for i, r := range key {
		if r == '-' {
			r = '_'
		}
		if i > 0 && isCredentialKeyBoundary(prev, r) {
			out = append(out, '_')
		}
		out = append(out, unicode.ToLower(r))
		prev = r
	}
	key = string(out)
	key = strings.ReplaceAll(key, "-", "_")
	return key
}

func isCredentialKeyBoundary(prev, current rune) bool {
	if prev == '_' || current == '_' {
		return false
	}
	return (unicode.IsLower(prev) || unicode.IsDigit(prev)) && unicode.IsUpper(current)
}

func isBenignTokenField(key string) bool {
	if isTokenMetricField(key) ||
		isTokenMetadataField(key) ||
		isResourceTokenField(key) ||
		isPaginationOrSyncTokenField(key) {
		return true
	}
	return false
}

func isTokenMetricField(key string) bool {
	switch key {
	case "tokenizer",
		"token_count",
		"tokens",
		"max_tokens",
		"completion_tokens",
		"prompt_tokens":
		return true
	default:
		return false
	}
}

func isTokenMetadataField(key string) bool {
	switch key {
	case "access_token_expires_in",
		"refresh_token_expires_in",
		"token_expires_in",
		"token_status",
		"token_type",
		"token_url",
		"token_endpoint",
		"token_format",
		"secret_name":
		return true
	default:
		return false
	}
}

func isPaginationOrSyncTokenField(key string) bool {
	switch key {
	case "page_token",
		"next_page_token",
		"sync_token":
		return true
	default:
		return false
	}
}

func isResourceTokenField(key string) bool {
	if !strings.HasSuffix(key, "_token") {
		return false
	}
	prefix := strings.TrimSuffix(key, "_token")
	switch prefix {
	case "app",
		"base",
		"board",
		"doc",
		"drive_route",
		"file",
		"folder",
		"host_node",
		"minute",
		"node",
		"obj",
		"origin_node",
		"parent",
		"parent_file",
		"parent_node",
		"share",
		"spreadsheet",
		"target",
		"wiki":
		return true
	default:
		return false
	}
}

func isResourceTokenPlaceholderAssignment(key, value string) bool {
	switch {
	case key == "client_token" && idempotencyTokenPlaceholderValue(value):
		return true
	case key == "retry_without_token" && numericStringPlaceholderValue(value):
		return true
	case tokenLikePlaceholderKey(key):
		return tokenLikePlaceholderValue(key, value)
	default:
		return false
	}
}

func tokenLikePlaceholderKey(key string) bool {
	return key == "token" ||
		strings.HasSuffix(key, "_token") ||
		strings.HasSuffix(key, "-token")
}

func tokenLikePlaceholderValue(key, value string) bool {
	normalized := strings.ToLower(strings.Trim(value, `"'`))
	if normalized == "" || credentialShapedIdentifier(normalized) {
		return false
	}
	if authCredentialTokenKey(key) {
		return false
	}
	return resourceTokenPlaceholderValue(value) ||
		maskedTokenFixturePlaceholderValue(key, normalized) ||
		isPlaceholderValue(value) ||
		normalized == "token" ||
		strings.Contains(normalized, "...") ||
		strings.Contains(normalized, "xxx") ||
		strings.Contains(normalized, "_or_") ||
		strings.HasSuffix(normalized, "_token") ||
		strings.HasPrefix(normalized, ".")
}

func maskedTokenFixturePlaceholderValue(key, value string) bool {
	if authCredentialTokenKey(key) {
		return false
	}
	var stars, alnum int
	for _, r := range value {
		switch {
		case r == '*':
			stars++
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			alnum++
		default:
			return false
		}
	}
	return stars >= 6 && alnum > 0
}

func isWeakTokenCredentialKey(key string) bool {
	if authCredentialTokenKey(key) || isStrongTokenCredentialKey(key) {
		return false
	}
	return key == "token" ||
		strings.HasSuffix(key, "_token") ||
		strings.HasSuffix(key, "-token")
}

func isStrongTokenCredentialKey(key string) bool {
	parts := credentialKeyParts(strings.ReplaceAll(strings.ToLower(key), "-", "_"))
	for _, phrase := range [][2]string{
		{"access", "token"},
		{"refresh", "token"},
		{"auth", "token"},
		{"bearer", "token"},
		{"session", "token"},
		{"service", "token"},
		{"bot", "token"},
		{"api", "token"},
		{"secret", "token"},
	} {
		if hasAdjacentCredentialParts(parts, phrase[0], phrase[1]) {
			return true
		}
	}
	return false
}

func weakTokenValueLooksCredentialLike(value string) bool {
	normalized := strings.ToLower(strings.Trim(value, `"'<>`))
	if normalized == "" ||
		isNonSecretLiteralValue(value) ||
		isPlaceholderValue(value) {
		return false
	}
	candidate := unwrapCredentialValue(normalized)
	return credentialShapedIdentifier(candidate) ||
		highEntropyCredentialValue(candidate) ||
		commandSubstitutionLooksCredentialLike(normalized) ||
		(strings.Contains(normalized, "://") &&
			urlRemainderLooksCredentialLike(removeAnglePlaceholders(normalized)))
}

func unwrapCredentialValue(value string) string {
	value = strings.TrimSpace(strings.Trim(value, `"'<>`))
	if strings.HasPrefix(value, "${{") && strings.HasSuffix(value, "}}") {
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "${{"), "}}"))
	}
	value = strings.TrimPrefix(value, "$")
	value = strings.Trim(value, "%")
	return strings.TrimSpace(value)
}

func highEntropyCredentialValue(value string) bool {
	if len(value) < 32 {
		return false
	}
	var hasLetter, hasDigit bool
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == '_' || r == '-' || r == '.' || r == '=':
		default:
			return false
		}
	}
	return hasLetter && hasDigit && shannonEntropy(value) >= 3.5
}

func shannonEntropy(value string) float64 {
	if value == "" {
		return 0
	}
	counts := map[rune]int{}
	for _, r := range value {
		counts[r]++
	}
	var entropy float64
	length := float64(len([]rune(value)))
	for _, count := range counts {
		p := float64(count) / length
		entropy -= p * log2(p)
	}
	return entropy
}

func log2(value float64) float64 {
	return math.Log(value) / math.Ln2
}

func authCredentialTokenKey(key string) bool {
	switch strings.ReplaceAll(strings.ToLower(key), "-", "_") {
	case "access_token",
		"api_token",
		"bot_token",
		"refresh_token",
		"secret_token",
		"session_token",
		"service_token",
		"bearer_token",
		"auth_token",
		"authorization_token",
		"id_token":
		return true
	default:
		return false
	}
}

func isPermissionScopeIdentifierAssignment(key, value string) bool {
	if !strings.HasSuffix(key, "_token") {
		return false
	}
	switch strings.ToLower(strings.Trim(value, `"',;`)) {
	case "read", "write", "modify", "readonly", "get_as_user":
		return true
	default:
		return false
	}
}

func idempotencyTokenPlaceholderValue(value string) bool {
	return numericStringPlaceholderValue(value) || uuidStringPlaceholderValue(value)
}

func uuidStringPlaceholderValue(value string) bool {
	normalized := strings.Trim(value, `"'`)
	parts := strings.Split(normalized, "-")
	if len(parts) != 5 {
		return false
	}
	for i, part := range parts {
		want := []int{8, 4, 4, 4, 12}[i]
		if len(part) != want {
			return false
		}
		for _, r := range part {
			if (r >= '0' && r <= '9') ||
				(r >= 'a' && r <= 'f') ||
				(r >= 'A' && r <= 'F') {
				continue
			}
			return false
		}
	}
	return true
}

func numericStringPlaceholderValue(value string) bool {
	normalized := strings.Trim(value, `"'`)
	if normalized == "" {
		return false
	}
	for _, r := range normalized {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isBenignCodeCredentialExpression(file, line, match, value string) bool {
	normalized := strings.TrimSpace(value)
	if strings.HasPrefix(normalized, "regexp.MustCompile(") {
		return true
	}
	if !sourceCodeFile(file) || credentialShapedValue(value) {
		return false
	}
	if rhs, ok := sourceCodeTypedCredentialRHS(line, match); ok {
		return isBenignTypedCredentialRHS(rhs)
	}
	rawValueQuoted := credentialAssignmentRawValueQuoted(match)
	if sourceCodeLiteralLooksNonSecret(normalized, !rawValueQuoted) {
		return true
	}
	if sourceCodeFormatStringLiteral(normalized) && sourceCodeFormatArgumentContext(line, match) {
		return true
	}
	if strings.Contains(match, "+") {
		return true
	}
	if rawValueQuoted {
		return false
	}
	if quotedLiteral(value) {
		return sourceCodeLiteralLooksNonSecret(value, false)
	}
	return codeReferenceExpression(normalized)
}

func sourceCodeTypedCredentialRHS(line, match string) (string, bool) {
	idx := strings.Index(line, match)
	if idx < 0 {
		return "", false
	}
	key, ok := credentialAssignmentKey(match)
	if !ok {
		return "", false
	}
	rest := strings.TrimSpace(line[idx+len(key):])
	if !strings.HasPrefix(rest, ":") {
		return "", false
	}
	typeAndRHS := strings.TrimSpace(strings.TrimPrefix(rest, ":"))
	assignmentIdx := strings.Index(typeAndRHS, "=")
	if assignmentIdx < 0 {
		return "", false
	}
	return strings.TrimSpace(typeAndRHS[assignmentIdx+1:]), true
}

func isBenignTypedCredentialRHS(value string) bool {
	value = strings.TrimRight(strings.TrimSpace(value), ",;")
	if value == "" || isNonSecretLiteralValue(value) || isPlaceholderValue(value) {
		return true
	}
	if credentialShapedValue(value) {
		return false
	}
	if sourceCodeLiteralLooksNonSecret(value, !quotedLiteral(value)) {
		return true
	}
	if quotedLiteral(value) {
		return false
	}
	return codeReferenceExpression(value)
}

func credentialAssignmentRawValueQuoted(match string) bool {
	key, ok := credentialAssignmentKey(match)
	if !ok {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(match[len(key):], ":"))
	rest = strings.TrimSpace(strings.TrimPrefix(rest, "="))
	return strings.HasPrefix(rest, `"`) || strings.HasPrefix(rest, `'`)
}

func sourceCodeFile(file string) bool {
	switch filepath.Ext(file) {
	case ".go", ".js", ".jsx", ".py", ".ts", ".tsx":
		return true
	default:
		return false
	}
}

func quotedLiteral(value string) bool {
	normalized := strings.TrimSpace(value)
	return len(normalized) >= 2 &&
		((strings.HasPrefix(normalized, `"`) && strings.HasSuffix(normalized, `"`)) ||
			(strings.HasPrefix(normalized, `'`) && strings.HasSuffix(normalized, `'`)))
}

func sourceCodeLiteralLooksNonSecret(value string, allowNumeric bool) bool {
	literal := strings.Trim(strings.TrimSpace(value), `"'`)
	if strings.HasPrefix(literal, "/") {
		return true
	}
	return (allowNumeric && numericStringPlaceholderValue(literal)) ||
		sourceCodeEnvVarNameLiteral(literal) ||
		sourceCodeAttributeNameLiteral(literal) ||
		sourceCodeFakeOrPlaceholderLiteral(literal) ||
		sourceCodeCredentialTermLiteral(literal) ||
		sourceCodeCredentialPrefixLiteral(literal) ||
		sourceCodeVocabularyLiteral(literal) ||
		sourceCodeSchemaTypeLiteral(literal) ||
		benignCredentialStatusLiteral(literal)
}

func sourceCodeFormatArgumentContext(line, match string) bool {
	idx := strings.Index(line, match)
	if idx < 0 {
		return false
	}
	prefix := line[:idx]
	if semicolon := strings.LastIndex(prefix, ";"); semicolon >= 0 {
		prefix = prefix[semicolon+1:]
	}
	return strings.Contains(prefix, "fmt.") ||
		strings.Contains(prefix, "log.") ||
		strings.Contains(prefix, "printf(") ||
		strings.Contains(prefix, "Printf(") ||
		strings.Contains(prefix, "Errorf(") ||
		strings.Contains(prefix, "Fprintf(")
}

func sourceCodeFormatStringLiteral(value string) bool {
	for i := 0; i < len(value)-1; i++ {
		if value[i] != '%' {
			continue
		}
		if value[i+1] == '%' {
			i++
			continue
		}
		j := i + 1
		for j < len(value) && strings.ContainsRune("#+- 0.0123456789", rune(value[j])) {
			j++
		}
		if j < len(value) && strings.ContainsRune("vTtbcdoOqxXUeEfFgGspw", rune(value[j])) {
			return true
		}
	}
	return false
}

func sourceCodeEnvVarNameLiteral(value string) bool {
	if value == "" || !strings.Contains(value, "_") {
		return false
	}
	var hasCredentialMarker bool
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	for _, marker := range []string{"TOKEN", "SECRET", "KEY", "PASSWORD", "PASSWD"} {
		if strings.Contains(value, marker) {
			hasCredentialMarker = true
			break
		}
	}
	return hasCredentialMarker
}

func sourceCodeAttributeNameLiteral(value string) bool {
	normalized := strings.ToLower(value)
	return strings.HasPrefix(normalized, "data-") && delimitedPlaceholderIdentifier(normalized)
}

func sourceCodeFakeOrPlaceholderLiteral(value string) bool {
	normalized := strings.ToLower(value)
	return strings.HasPrefix(normalized, "fake_") ||
		strings.HasPrefix(normalized, "fake-") ||
		strings.Contains(normalized, "placeholder") ||
		(strings.Contains(normalized, "<") && strings.Contains(normalized, ">"))
}

func sourceCodeCredentialTermLiteral(value string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(value, "-", "_"))
	return conventionalCredentialPlaceholderName(normalized)
}

func sourceCodeCredentialPrefixLiteral(value string) bool {
	switch strings.ToLower(value) {
	case "appsecret:":
		return true
	default:
		return false
	}
}

func sourceCodeVocabularyLiteral(value string) bool {
	switch strings.ToLower(value) {
	case "bot", "tenant", "user":
		return true
	default:
		return false
	}
}

func sourceCodeSchemaTypeLiteral(value string) bool {
	normalized := strings.ToLower(value)
	return normalized == "string" || strings.HasPrefix(normalized, "string(")
}

func benignCredentialStatusLiteral(value string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(value, "-", "_"))
	if !delimitedPlaceholderIdentifier(normalized) {
		return false
	}
	for _, marker := range []string{
		"bad_fmt",
		"expired",
		"format",
		"invalid",
		"missing",
		"permission",
		"status",
		"type",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func codeReferenceExpression(value string) bool {
	value = strings.TrimRight(strings.TrimSpace(value), ";")
	if value == "" {
		return false
	}
	for _, marker := range []string{".", "(", ")", "[", "]", "{"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	if !codeIdentifier(value) {
		return false
	}
	return codeIdentifier(value)
}

func codeIdentifier(value string) bool {
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '_' && i > 0:
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func isNonSecretLiteralValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(strings.Trim(value, `"'`))) {
	case "true", "false", "null", "nil", "{", "[":
		return true
	default:
		return false
	}
}

func isJWTToken(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	header, err := decodeBase64URLSegment(parts[0])
	if err != nil || !json.Valid(header) {
		return false
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(header, &fields); err != nil {
		return false
	}
	alg, ok := fields["alg"].(string)
	return ok && alg != ""
}

func decodeBase64URLSegment(value string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(value)
}

func isPlaceholderBearerHeader(match string) bool {
	normalized := strings.ToLower(match)
	idx := strings.LastIndex(normalized, "bearer ")
	if idx < 0 {
		return false
	}
	value := strings.TrimSpace(match[idx+len("bearer "):])
	return isPlaceholderValue(value)
}

func isWebhookCredentialKey(key string) bool {
	return strings.Contains(strings.ReplaceAll(key, "_", ""), "webhook")
}

func webhookAssignmentValueLooksCredentialLike(value string) bool {
	normalized := strings.ToLower(strings.Trim(value, `"'`))
	if normalized == "" || isPlaceholderValue(normalized) || isNonSecretLiteralValue(normalized) {
		return false
	}
	return urlRemainderLooksCredentialLike(removeAnglePlaceholders(normalized)) ||
		credentialShapedIdentifier(strings.Trim(normalized, "$"))
}

func isExplicitCredentialKey(key string) bool {
	compact := strings.ReplaceAll(key, "_", "")
	switch compact {
	case "token",
		"accesstoken",
		"refreshtoken",
		"authtoken",
		"bearertoken",
		"sessiontoken",
		"servicetoken",
		"apikey",
		"accesskey",
		"privatekey",
		"apisecret",
		"secret",
		"secretkey",
		"clientsecret",
		"password",
		"passwd":
		return true
	}
	for _, phrase := range []string{
		"accesstoken",
		"refreshtoken",
		"authtoken",
		"bearertoken",
		"sessiontoken",
		"servicetoken",
		"bottoken",
		"apikey",
		"accesskey",
		"privatekey",
		"apisecret",
		"clientsecret",
		"secretkey",
	} {
		if strings.Contains(compact, phrase) {
			return true
		}
	}
	parts := credentialKeyParts(key)
	for _, phrase := range [][2]string{
		{"access", "token"},
		{"refresh", "token"},
		{"auth", "token"},
		{"bearer", "token"},
		{"session", "token"},
		{"service", "token"},
		{"bot", "token"},
		{"api", "key"},
		{"access", "key"},
		{"private", "key"},
		{"api", "secret"},
		{"client", "secret"},
		{"secret", "key"},
	} {
		if hasAdjacentCredentialParts(parts, phrase[0], phrase[1]) {
			return true
		}
	}
	for _, part := range parts {
		switch part {
		case "token", "secret", "password", "passwd":
			return true
		}
	}
	for _, suffix := range []string{
		"token",
		"accesstoken",
		"refreshtoken",
		"authtoken",
		"bearertoken",
		"sessiontoken",
		"servicetoken",
		"bottoken",
		"apikey",
		"accesskey",
		"privatekey",
		"apisecret",
		"clientsecret",
		"secret",
		"secretkey",
		"password",
		"passwd",
	} {
		if strings.HasSuffix(compact, suffix) {
			return true
		}
	}
	for _, suffix := range []string{
		"_access_token",
		"_refresh_token",
		"_auth_token",
		"_bearer_token",
		"_session_token",
		"_service_token",
		"_api_key",
		"_access_key",
		"_private_key",
		"_api_secret",
		"_client_secret",
		"_secret",
		"_secret_key",
		"_password",
		"_passwd",
	} {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	return false
}

func credentialKeyParts(key string) []string {
	var parts []string
	for _, part := range strings.Split(key, "_") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func hasAdjacentCredentialParts(parts []string, first, second string) bool {
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == first && parts[i+1] == second {
			return true
		}
	}
	return false
}

func credentialAssignmentValue(match []string) string {
	for _, value := range match[1:] {
		if value != "" {
			return value
		}
	}
	return ""
}

func looksLikeEqualityComparison(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "=")
}

func isPlaceholderCredentialURL(file, raw string) bool {
	userInfo, ok := credentialURLUserInfo(raw)
	if !ok {
		return false
	}
	_, password, ok := strings.Cut(userInfo, ":")
	if !ok {
		return false
	}
	return credentialURLPasswordPlaceholder(password) ||
		(sourceOrTestFixtureFile(file) && credentialURLPasswordFixture(password))
}

func credentialURLPasswordPlaceholder(password string) bool {
	normalized := strings.ToLower(password)
	decoded := strings.ReplaceAll(normalized, "%3c", "<")
	decoded = strings.ReplaceAll(decoded, "%3e", ">")
	switch decoded {
	case "placeholder", "redacted", "<redacted>", "xxxx":
		return true
	}
	return angleWrappedPlaceholder(decoded) || percentWrappedPlaceholder(decoded)
}

func credentialURLPasswordFixture(password string) bool {
	normalized := strings.ToLower(strings.Trim(password, `"'`))
	switch normalized {
	case "p",
		"pass",
		"password",
		"pat_abc",
		"pw",
		"s3cret",
		"secret",
		"t":
		return true
	default:
		return false
	}
}

func sourceOrTestFixtureFile(file string) bool {
	normalized := filepath.ToSlash(file)
	return sourceCodeFile(normalized) ||
		strings.HasPrefix(normalized, "testdata/") ||
		strings.HasPrefix(normalized, "fixtures/") ||
		strings.Contains(normalized, "/testdata/") ||
		strings.Contains(normalized, "/fixtures/")
}

func warnForPrivateIPv4(file string) bool {
	normalized := filepath.ToSlash(file)
	if sourceOrTestFixtureFile(normalized) {
		return false
	}
	switch filepath.Ext(normalized) {
	case ".md", ".mdx", ".txt", ".json", ".yaml", ".yml", ".toml", ".env":
		return true
	default:
		return strings.HasPrefix(normalized, "docs/") ||
			strings.HasPrefix(normalized, "skills/")
	}
}

func credentialURLUserInfo(raw string) (string, bool) {
	schemeIdx := strings.Index(raw, "://")
	if schemeIdx < 0 {
		return "", false
	}
	rest := raw[schemeIdx+len("://"):]
	atIdx := strings.Index(rest, "@")
	if atIdx < 0 {
		return "", false
	}
	return rest[:atIdx], true
}

func newFinding(rule, file string, line int, source, excerpt string) Finding {
	return Finding{
		Rule:       rule,
		Action:     actionForRule(rule),
		File:       file,
		Line:       line,
		Source:     source,
		Excerpt:    excerpt,
		Message:    messageForRule(rule),
		Suggestion: suggestionForRule(rule),
	}
}

func messageForRule(rule string) string {
	switch rule {
	case "public_content_generic_credential":
		return "public contribution contains a generic credential assignment"
	case "public_content_private_key_block":
		return "public contribution contains a private key block"
	case "public_content_jwt_like_token":
		return "public contribution contains a JWT-like token"
	case "public_content_bearer_header":
		return "public contribution contains an Authorization bearer token"
	case "public_content_credential_url":
		return "public contribution contains credentials embedded in a URL"
	case "public_content_private_ipv4":
		return "public contribution contains a private-network IP address"
	case "public_content_automation_branch":
		return "public contribution uses an automation-shaped branch name"
	case "public_content_change_id_trailer":
		return "public contribution contains a Change-Id trailer"
	case "public_content_reviewed_on_trailer":
		return "public contribution contains a Reviewed-on trailer"
	case "public_content_provenance_marker":
		return "public contribution contains a prohibited provenance marker"
	case "public_content_detector_fingerprint":
		return "public rule/config content exposes public detector fingerprints"
	case "public_content_harness_metadata":
		return "public contribution contains visible harness pipeline metadata"
	case "public_content_ccm_harness_trailer":
		return "public contribution contains a CCM-Harness trailer"
	case "public_content_semantic_candidate":
		return "public contribution contains text for semantic public content review"
	default:
		return "public contribution contains content that should not be published"
	}
}

func suggestionForRule(rule string) string {
	switch actionForRule(rule) {
	case "REJECT":
		return "remove the value from the public contribution and replace it with a non-sensitive placeholder"
	default:
		return "remove private workflow metadata before publishing the public contribution"
	}
}

func redactAssignment(match string) string {
	key, ok := credentialAssignmentKey(match)
	if !ok {
		return "<credential-assignment>"
	}
	return fmt.Sprintf("%s= <redacted>", strings.TrimSpace(key))
}

func credentialAssignmentKey(match string) (string, bool) {
	idx := -1
	for _, sep := range []string{":", "="} {
		if candidate := strings.Index(match, sep); candidate >= 0 && (idx < 0 || candidate < idx) {
			idx = candidate
		}
	}
	if idx < 0 {
		return "", false
	}
	return match[:idx], true
}

func redactToken(_ string) string {
	return "<jwt-like-token>"
}

func redactedSemanticExcerpt(text string) string {
	normalized := strings.Join(strings.Fields(text), " ")
	if normalized == "" {
		return ""
	}
	signals := semanticSignals(normalized)
	if len(signals) == 0 {
		return ""
	}
	sanitized := truncateRunes(sanitizeSemanticExcerpt(text), 600)
	return fmt.Sprintf("semantic signals: %s; excerpt: %q", strings.Join(signals, ","), sanitized)
}

func semanticSignals(normalized string) []string {
	lower := strings.ToLower(normalized)
	var signals []string
	add := func(signal string) {
		for _, existing := range signals {
			if existing == signal {
				return
			}
		}
		signals = append(signals, signal)
	}

	hasPrivateScope := strings.Contains(lower, "private") || strings.Contains(lower, "internal-only")
	hasRequestMetadata := strings.Contains(lower, "request header") || strings.Contains(lower, "request headers") || strings.Contains(lower, "authorization header") || strings.Contains(lower, "metadata header")
	hasTrustBoundary := strings.Contains(lower, "spoof") || strings.Contains(lower, "trust") || strings.Contains(lower, "risk scoring") || strings.Contains(lower, "classification")
	hasRoadmap := strings.Contains(lower, "roadmap") || strings.Contains(lower, "migration") || strings.Contains(lower, "rollout") || strings.Contains(lower, "cutover") || strings.Contains(lower, "unpublished")
	hasTiming := strings.Contains(lower, "target date") || strings.Contains(lower, "friday") || strings.Contains(lower, "monday") || strings.Contains(lower, "tuesday") || strings.Contains(lower, "wednesday") || strings.Contains(lower, "thursday") || strings.Contains(lower, "customer-visible")
	hasImplementation := strings.Contains(lower, "server-side") || strings.Contains(lower, "implementation")

	if hasPrivateScope && hasRequestMetadata && hasTrustBoundary {
		add("private_scope")
		add("request_metadata")
		add("trust_boundary_detail")
	}
	if hasRoadmap && (hasPrivateScope || hasTiming) {
		add("roadmap_detail")
		if hasPrivateScope {
			add("private_scope")
		}
		if hasTiming {
			add("roadmap_timing")
		}
	}
	if hasPrivateScope && hasImplementation && hasTrustBoundary {
		add("private_scope")
		add("implementation_detail")
		add("trust_boundary_detail")
	}

	return signals
}

func sanitizeSemanticExcerpt(text string) string {
	text = redactPrivateKeyBlocks(text)
	text = credentialAssignmentRE.ReplaceAllStringFunc(text, sanitizeCredentialAssignment)
	text = strings.ReplaceAll(text, `<redacted>"`, `<redacted>`)
	text = strings.ReplaceAll(text, `<redacted>'`, `<redacted>`)
	text = semanticBearerHeaderRE.ReplaceAllString(text, "Authorization: Bearer <redacted>")
	text = jwtLikeRE.ReplaceAllStringFunc(text, func(match string) string {
		if isJWTToken(match) {
			return "<jwt-like-token>"
		}
		return match
	})
	text = credentialURLRE.ReplaceAllStringFunc(text, sanitizeCredentialURL)
	return strings.Join(strings.Fields(text), " ")
}

func redactPrivateKeyBlocks(text string) string {
	lines := strings.Split(text, "\n")
	var out []string
	inPrivateKey := false
	for _, line := range lines {
		if strings.Contains(line, privateKeyBeginPrefix) && strings.Contains(line, privateKeyMarker) {
			out = append(out, "<private-key-block>")
			inPrivateKey = true
			if strings.Contains(line, privateKeyEndPrefix) && strings.Contains(line, privateKeyMarker) {
				inPrivateKey = false
			}
			continue
		}
		if inPrivateKey {
			if strings.Contains(line, privateKeyEndPrefix) && strings.Contains(line, privateKeyMarker) {
				inPrivateKey = false
			}
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func sanitizeCredentialAssignment(match string) string {
	key, ok := credentialAssignmentKey(match)
	if !ok {
		return "<credential-assignment>"
	}
	return strings.TrimSpace(key) + "=<redacted>"
}

func sanitizeCredentialURL(raw string) string {
	redacted := redactCredentialURL(raw)
	redacted = strings.ReplaceAll(redacted, "%3Cuser%3E", "<user>")
	redacted = strings.ReplaceAll(redacted, "%3Credacted%3E", "<redacted>")
	return redacted
}

func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}
