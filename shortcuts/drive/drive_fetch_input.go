// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"net/url"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// driveFetchInput is the parsed --url / --token+--type input. inputType is the
// normalized dispatch type (doc/docx/sheet/bitable/slides/file/minutes) for non-wiki
// input, or "wiki" when the input is a wiki node (unwrapped at execute time).
type driveFetchInput struct {
	rawURL      string            // original URL input ("" for bare token)
	inputType   string            // normalized dispatch type, or "wiki"
	token       string            // the resource/node token
	selector    map[string]string // curated ?sheet=/?table= for the envelope
	query       string            // full RawQuery, reattached to rebuilt URLs
	isBareToken bool
}

// resolveDriveFetchInput parses --url / --token+--type into a driveFetchInput.
// A URL is auto-detected (--type, if given, must match); a bare token requires
// --type. doc/docx remain distinct; sheets→sheet, base→bitable.
// Wiki URLs (and --type wiki) stay "wiki" — the underlying type is resolved at
// execute via ResolveWikiNode.
func resolveDriveFetchInput(runtime *common.RuntimeContext) (driveFetchInput, error) {
	rawURL := strings.TrimSpace(runtime.Str("url"))
	tokenFlag := strings.TrimSpace(runtime.Str("token"))
	inputType := strings.ToLower(strings.TrimSpace(runtime.Str("type")))

	if rawURL != "" && tokenFlag != "" {
		return driveFetchInput{}, common.ValidationErrorf("pass either --url or --token, not both").WithParam("--url")
	}
	if rawURL == "" && tokenFlag == "" {
		return driveFetchInput{}, common.ValidationErrorf("one of --url or --token is required").WithParam("--url")
	}

	if rawURL != "" {
		u, perr := url.Parse(rawURL)
		if perr != nil || u.Path == "" {
			return driveFetchInput{}, common.ValidationErrorf("--url %q is not a recognized Lark resource URL (docx, doc, sheet, base, wiki, slides, file, minutes)", rawURL).WithParam("--url")
		}
		// minutes URLs (https://meetings.feishu.cn/minutes/<token>) are not in
		// ParseResourceURL's table (adding them there would change drive +inspect's
		// rejection of minutes links); detect them here so --url accepts a minutes link.
		var urlType, token string
		if t, ok := minutesURLTokenFromURL(u); ok {
			urlType, token = "minutes", t
		} else {
			ref, ok := common.ParseResourceURL(rawURL)
			if !ok {
				return driveFetchInput{}, common.ValidationErrorf("--url %q is not a recognized Lark resource URL (docx, doc, sheet, base, wiki, slides, file, minutes)", rawURL).WithParam("--url")
			}
			nt, ok := normalizeFetchType(ref.Type)
			if !ok {
				return driveFetchInput{}, common.ValidationErrorf("--url %q is not a fetchable resource type (got %q)", rawURL, ref.Type).WithParam("--url")
			}
			urlType, token = nt, ref.Token
		}
		if inputType != "" {
			declared, ok := normalizeFetchType(inputType)
			if !ok {
				return driveFetchInput{}, common.ValidationErrorf("--type %q is not a recognized fetch type", inputType).WithParam("--type")
			}
			if declared != urlType {
				return driveFetchInput{}, common.ValidationErrorf("--type %q conflicts with URL type %q; remove --type or use a matching value", inputType, urlType).WithParam("--type")
			}
		}
		return driveFetchInput{
			rawURL:    rawURL,
			inputType: urlType,
			token:     token,
			selector:  captureSelector(u),
			query:     captureQuery(u),
		}, nil
	}

	// bare token
	if inputType == "" {
		return driveFetchInput{}, common.ValidationErrorf("--type is required with --token (allowed: doc, docx, sheet, base, bitable, slides, file, minutes, wiki)").WithParam("--type")
	}
	normalized, ok := normalizeFetchType(inputType)
	if !ok {
		return driveFetchInput{}, common.ValidationErrorf("--type %q is not a recognized fetch type (allowed: doc, docx, sheets, base, bitable, slides, file, minutes, wiki)", inputType).WithParam("--type")
	}
	if strings.ContainsAny(tokenFlag, "/?#") {
		return driveFetchInput{}, common.ValidationErrorf("--token %q must be a bare token (no path/query/fragment)", tokenFlag).WithParam("--token")
	}
	return driveFetchInput{
		inputType:   normalized,
		token:       tokenFlag,
		isBareToken: true,
	}, nil
}

// normalizeFetchType canonicalizes aliases while preserving distinct resource
// types. sheets → "sheet"; base/bitable → "bitable". Returns ok=false for
// non-fetchable types (mindnote, folder, unknown).
func normalizeFetchType(t string) (string, bool) {
	switch t {
	case "doc", "docx":
		return t, true
	case "sheet", "sheets":
		return "sheet", true
	case "base", "bitable":
		return "bitable", true
	case "slides":
		return "slides", true
	case "file":
		return "file", true
	case "minutes":
		return "minutes", true
	case "wiki":
		return "wiki", true
	}
	return "", false
}

// captureSelector extracts the curated ?sheet=/?table=/?view= sub-resource
// selectors for the envelope's resource.selector. Returns nil when none are
// present.
func captureSelector(u *url.URL) map[string]string {
	if u == nil {
		return nil
	}
	q := u.Query()
	sel := map[string]string{}
	for _, k := range []string{"sheet", "table", "view"} {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			sel[k] = v
		}
	}
	if len(sel) == 0 {
		return nil
	}
	return sel
}

// captureQuery returns the URL's full RawQuery (to reattach to rebuilt URLs so
// the fetch service sees the same selectors it would from a verbatim URL).
func captureQuery(u *url.URL) string {
	if u == nil {
		return ""
	}
	return strings.TrimSpace(u.RawQuery)
}

// minutesURLTokenFromURL extracts the minute token from a
// https://meetings.feishu.cn/minutes/<token> URL. Minutes URLs are not in
// ParseResourceURL's table (adding them there would change drive +inspect's
// rejection of minutes links), so drive +fetch detects them here to accept a
// minutes link via --url. Returns ok=false when the path is not /minutes/<token>.
func minutesURLTokenFromURL(u *url.URL) (string, bool) {
	if u == nil || !strings.HasPrefix(u.Path, "/minutes/") {
		return "", false
	}
	rest := strings.TrimRight(u.Path[len("/minutes/"):], "/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", false
	}
	return rest, true
}
