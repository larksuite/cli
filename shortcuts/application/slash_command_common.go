// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
)

// slashCommandBasePath is the raw v7 endpoint (not in meta_data.json / SDK).
const slashCommandBasePath = "/open-apis/application/v7/app_slash_commands"

// clientCacheHint is printed to stderr after every successful write.
const clientCacheHint = "note: changes take ~5 minutes to appear in Feishu clients (client-side cache); the server state is already updated - list reflects it immediately."

// parseDescriptionI18n parses repeated --description-i18n values ("<lang>=<text>",
// split on the FIRST '='). Returns nil for empty input. Duplicate langs rejected.
func parseDescriptionI18n(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	m := make(map[string]string, len(values))
	for _, v := range values {
		idx := strings.Index(v, "=")
		if idx <= 0 || idx == len(v)-1 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"invalid --description-i18n value %q: expected <lang>=<text> (e.g. zh_cn=你好)", v).
				WithParam("--description-i18n")
		}
		lang := strings.TrimSpace(v[:idx])
		text := v[idx+1:]
		if lang == "" || strings.TrimSpace(text) == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"invalid --description-i18n value %q: language and text must be non-empty", v).
				WithParam("--description-i18n")
		}
		if _, dup := m[lang]; dup {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"duplicate language %q in --description-i18n", lang).
				WithParam("--description-i18n")
		}
		m[lang] = text
	}
	return m, nil
}

// validateCommandName rejects empty and slash-prefixed command names.
func validateCommandName(name, flagName string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"%s must not be empty", flagName).WithParam(flagName)
	}
	if strings.HasPrefix(trimmed, "/") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"%s must not start with \"/\" - the slash is implied (use %q)",
			flagName, strings.TrimPrefix(trimmed, "/")).WithParam(flagName)
	}
	return nil
}

// encodeCommandIDPathSegment applies the same normalization and escaping to
// command IDs in dry-run output and real requests.
func encodeCommandIDPathSegment(id string) string {
	return validate.EncodePathSegment(strings.TrimSpace(id))
}

// buildSlashCommandBody assembles a create/update request body. Only provided
// fields are included: PATCH is field-level partial (absent top-level fields
// are preserved server-side; a provided i18n map REPLACES the whole map).
// icon sits at the top level, sibling of description (verified live; the
// official create sample nesting icon inside description is a doc bug).
func buildSlashCommandBody(command, description string, i18n map[string]string, iconKey string) map[string]interface{} {
	body := map[string]interface{}{}
	if command != "" {
		body["command"] = command
	}
	if description != "" || len(i18n) > 0 {
		desc := map[string]interface{}{}
		if description != "" {
			desc["default_value"] = description
		}
		if len(i18n) > 0 {
			desc["i18n"] = i18n
		}
		body["description"] = desc
	}
	if iconKey != "" {
		body["icon"] = map[string]interface{}{"icon_key": iconKey}
	}
	return body
}

// isCommandExists reports whether err is the server-side name-collision error
// (code=40000000, message contains "command already exists"; verified live).
func isCommandExists(err error) bool {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return false
	}
	return p.Code == 40000000 && strings.Contains(p.Message, "command already exists")
}
