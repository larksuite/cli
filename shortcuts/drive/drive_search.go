// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// driveSearchErrUserNotVisible is the Lark service code returned by
// doc_wiki/search when an open_id referenced in --creator-ids / --sharer-ids
// falls outside the app's user-visibility scope (different from the
// search:docs:read API scope).
const driveSearchErrUserNotVisible = 99992351

// open_time has a server-side cap of 3 months per request. Rather than
// reject or silently clamp, we narrow this request to the most recent
// 3-month slice and list the remaining slices in a stderr notice so the
// agent can re-invoke for older ranges.
const (
	driveSearchSliceDays         = 90  // one slice = server-side 3-month cap
	driveSearchMaxOpenedSpanDays = 365 // hard cap: reject --opened-* spans beyond ~1 year
)

var driveSearchSortValues = []string{
	"default",
	"edit_time",
	"edit_time_asc",
	"open_time",
	"create_time",
}

var driveSearchDocTypeSet = map[string]struct{}{
	"DOC": {}, "SHEET": {}, "BITABLE": {}, "MINDNOTE": {}, "FILE": {},
	"WIKI": {}, "DOCX": {}, "FOLDER": {}, "CATALOG": {}, "SLIDES": {}, "SHORTCUT": {},
}

// driveSearchHourAggregatedFields lists filter keys the server aggregates at
// hour granularity. We pre-snap start/end and emit a stderr notice so callers
// see what was sent and why.
var driveSearchHourAggregatedFields = map[string]struct{}{
	"my_edit_time":    {},
	"my_comment_time": {},
}

// Server caps list filters at 20 entries each. We reject above-cap input
// locally so users and agents get a named-flag error instead of an opaque
// server-side failure or truncated result.
const (
	driveSearchMaxChatIDs   = 20
	driveSearchMaxSharerIDs = 20
)

// DriveSearch searches docs/wikis via the v2 doc_wiki/search API using flat
// flags instead of a nested JSON filter, which is friendlier for AI agents and
// `--help` readers.
var DriveSearch = common.Shortcut{
	Service:     "drive",
	Command:     "+search",
	Description: "Search Lark docs, Wiki, and spreadsheet files with flat filters (Search v2: doc_wiki/search)",
	Risk:        "read",
	Scopes:      []string{"search:docs:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "query", Desc: "search keyword (may be empty to browse by filter only)"},

		{Name: "mine", Type: "bool", Desc: "restrict to docs I created (uses current user's open_id)"},
		{Name: "creator-ids", Desc: "comma-separated creator open_ids; mutually exclusive with --mine"},

		{Name: "edited-since", Desc: "start of [my edited] time window (e.g. 7d, 1m, 1y, 2026-04-01, RFC3339, unix seconds)"},
		{Name: "edited-until", Desc: "end of [my edited] time window"},
		{Name: "commented-since", Desc: "start of [my commented] time window"},
		{Name: "commented-until", Desc: "end of [my commented] time window"},
		{Name: "opened-since", Desc: "start of [my opened] time window"},
		{Name: "opened-until", Desc: "end of [my opened] time window"},
		{Name: "created-since", Desc: "start of [document created] time window"},
		{Name: "created-until", Desc: "end of [document created] time window"},

		{Name: "doc-types", Desc: "comma-separated types: doc,sheet,bitable,mindnote,file,wiki,docx,folder,catalog,slides,shortcut"},
		{Name: "folder-tokens", Desc: "comma-separated folder tokens (doc-only; mutually exclusive with --space-ids)"},
		{Name: "space-ids", Desc: "comma-separated wiki space IDs (wiki-only; mutually exclusive with --folder-tokens)"},
		{Name: "chat-ids", Desc: "comma-separated chat IDs"},
		{Name: "sharer-ids", Desc: "comma-separated sharer open_ids"},

		{Name: "only-title", Type: "bool", Desc: "match titles only"},
		{Name: "only-comment", Type: "bool", Desc: "search comments only"},
		{Name: "sort", Desc: "sort type", Enum: driveSearchSortValues},

		{Name: "page-token", Desc: "pagination token from a previous response"},
		{Name: "page-size", Default: "15", Desc: "page size (1-20, default 15)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateDriveSearchIDs(readDriveSearchSpec(runtime))
	},
	Tips: []string{
		"Time flags accept relative (e.g. 7d, 1m, 1y), absolute (2026-04-01, RFC3339), or unix seconds.",
		"my_edit_time and my_comment_time are hour-aggregated server-side; sub-hour inputs are snapped and a notice is printed to stderr.",
		"Use --mine for a quick \"docs I created\" filter. For other people, use --creator-ids ou_xxx,ou_yyy.",
		"--folder-tokens limits to doc-only search; --space-ids limits to wiki-only. They cannot be combined.",
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := readDriveSearchSpec(runtime)
		reqBody, notices, err := buildDriveSearchRequest(spec, runtime.UserOpenId(), time.Now())
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		for _, n := range notices {
			fmt.Fprintln(runtime.IO().ErrOut, n)
		}
		return common.NewDryRunAPI().
			POST("/open-apis/search/v2/doc_wiki/search").
			Body(reqBody)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := readDriveSearchSpec(runtime)
		reqBody, notices, err := buildDriveSearchRequest(spec, runtime.UserOpenId(), time.Now())
		if err != nil {
			return err
		}
		for _, n := range notices {
			fmt.Fprintln(runtime.IO().ErrOut, n)
		}

		data, err := callDriveSearchAPI(runtime, reqBody)
		if err != nil {
			return err
		}
		items, _ := data["res_units"].([]interface{})
		normalizedItems := addDriveSearchIsoTimeFields(items)

		resultData := map[string]interface{}{
			"total":      data["total"],
			"has_more":   data["has_more"],
			"page_token": data["page_token"],
			"results":    normalizedItems,
		}

		runtime.OutFormat(resultData, &output.Meta{Count: len(normalizedItems)}, func(w io.Writer) {
			renderDriveSearchTable(w, data, normalizedItems)
		})
		return nil
	},
}

// driveSearchSpec is the parsed flag set for a single +search invocation.
type driveSearchSpec struct {
	Query     string
	PageToken string
	PageSize  string

	Mine       bool
	CreatorIDs []string

	EditedSince    string
	EditedUntil    string
	CommentedSince string
	CommentedUntil string
	OpenedSince    string
	OpenedUntil    string
	CreatedSince   string
	CreatedUntil   string

	DocTypes     []string
	FolderTokens []string
	SpaceIDs     []string
	ChatIDs      []string
	SharerIDs    []string

	OnlyTitle   bool
	OnlyComment bool
	Sort        string
}

func readDriveSearchSpec(runtime *common.RuntimeContext) driveSearchSpec {
	return driveSearchSpec{
		Query:     runtime.Str("query"),
		PageToken: runtime.Str("page-token"),
		PageSize:  runtime.Str("page-size"),

		Mine:       runtime.Bool("mine"),
		CreatorIDs: common.SplitCSV(runtime.Str("creator-ids")),

		EditedSince:    runtime.Str("edited-since"),
		EditedUntil:    runtime.Str("edited-until"),
		CommentedSince: runtime.Str("commented-since"),
		CommentedUntil: runtime.Str("commented-until"),
		OpenedSince:    runtime.Str("opened-since"),
		OpenedUntil:    runtime.Str("opened-until"),
		CreatedSince:   runtime.Str("created-since"),
		CreatedUntil:   runtime.Str("created-until"),

		DocTypes:     upperAll(common.SplitCSV(runtime.Str("doc-types"))),
		FolderTokens: common.SplitCSV(runtime.Str("folder-tokens")),
		SpaceIDs:     common.SplitCSV(runtime.Str("space-ids")),
		ChatIDs:      common.SplitCSV(runtime.Str("chat-ids")),
		SharerIDs:    common.SplitCSV(runtime.Str("sharer-ids")),

		OnlyTitle:   runtime.Bool("only-title"),
		OnlyComment: runtime.Bool("only-comment"),
		Sort:        strings.TrimSpace(runtime.Str("sort")),
	}
}

// buildDriveSearchRequest turns the parsed spec into the API request body and a
// list of stderr notices (e.g. hour-snap adjustments). It does all validation
// that depends on the combination of flag values.
func buildDriveSearchRequest(spec driveSearchSpec, userOpenID string, now time.Time) (map[string]interface{}, []string, error) {
	if spec.Mine && len(spec.CreatorIDs) > 0 {
		return nil, nil, output.ErrValidation("cannot combine --mine and --creator-ids")
	}
	if len(spec.FolderTokens) > 0 && len(spec.SpaceIDs) > 0 {
		return nil, nil, output.ErrValidation("cannot combine --folder-tokens and --space-ids; doc and wiki scoped search cannot be combined")
	}
	if spec.Mine && userOpenID == "" {
		return nil, nil, output.ErrValidation("--mine requires a logged-in user open_id, but none is configured; run `lark-cli auth login` or set user open_id in config")
	}

	if err := validateDocTypes(spec.DocTypes); err != nil {
		return nil, nil, err
	}

	pageSize, err := parseDriveSearchPageSize(spec.PageSize)
	if err != nil {
		return nil, nil, err
	}

	request := map[string]interface{}{
		"query":     spec.Query,
		"page_size": pageSize,
	}
	if spec.PageToken != "" {
		request["page_token"] = spec.PageToken
	}

	filter := map[string]interface{}{}
	var notices []string

	// open_time is capped at 3 months server-side; if the user's window is
	// longer, narrow this request and emit a notice with the remaining slices.
	if n, err := clampOpenedTimeWindow(&spec, now); err != nil {
		return nil, nil, err
	} else if n != "" {
		notices = append(notices, n)
	}

	// Creator identity.
	switch {
	case spec.Mine:
		filter["creator_ids"] = []string{userOpenID}
	case len(spec.CreatorIDs) > 0:
		filter["creator_ids"] = spec.CreatorIDs
	}

	// Time dimensions — each fills at most one filter key; hour-aggregated ones
	// also contribute notices.
	timeDims := []struct {
		key        string
		since, til string
	}{
		{"my_edit_time", spec.EditedSince, spec.EditedUntil},
		{"my_comment_time", spec.CommentedSince, spec.CommentedUntil},
		{"open_time", spec.OpenedSince, spec.OpenedUntil},
		{"create_time", spec.CreatedSince, spec.CreatedUntil},
	}
	for _, d := range timeDims {
		rng, dimNotices, err := buildTimeRangeFilter(d.key, d.since, d.til, now)
		if err != nil {
			return nil, nil, err
		}
		if rng != nil {
			filter[d.key] = rng
		}
		notices = append(notices, dimNotices...)
	}

	// Scalar scope filters.
	if len(spec.DocTypes) > 0 {
		filter["doc_types"] = spec.DocTypes
	}
	if len(spec.ChatIDs) > 0 {
		filter["chat_ids"] = spec.ChatIDs
	}
	if len(spec.SharerIDs) > 0 {
		filter["sharer_ids"] = spec.SharerIDs
	}
	if spec.OnlyTitle {
		filter["only_title"] = true
	}
	if spec.OnlyComment {
		filter["only_comment"] = true
	}
	if spec.Sort != "" {
		// Server enum uses "DEFAULT_TYPE" for the default sort; every other
		// value upper-cases 1:1.
		sortType := strings.ToUpper(spec.Sort)
		if sortType == "DEFAULT" {
			sortType = "DEFAULT_TYPE"
		}
		filter["sort_type"] = sortType
	}

	// Wiki-/folder-scoped variants: keep the shared filter, then add the
	// scope-specific key only into the correct side.
	switch {
	case len(spec.FolderTokens) > 0:
		docFilter := cloneDriveSearchFilter(filter)
		docFilter["folder_tokens"] = spec.FolderTokens
		request["doc_filter"] = docFilter
	case len(spec.SpaceIDs) > 0:
		wikiFilter := cloneDriveSearchFilter(filter)
		wikiFilter["space_ids"] = spec.SpaceIDs
		request["wiki_filter"] = wikiFilter
	default:
		request["doc_filter"] = cloneDriveSearchFilter(filter)
		request["wiki_filter"] = cloneDriveSearchFilter(filter)
	}

	return request, notices, nil
}

func parseDriveSearchPageSize(raw string) (int, error) {
	if raw == "" {
		return 15, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, output.ErrValidation("--page-size must be a number, got %q", raw)
	}
	if n <= 0 {
		return 15, nil
	}
	if n > 20 {
		n = 20
	}
	return n, nil
}

// validateDriveSearchIDs checks open_id / chat_id format and enforces the
// 20-entry cap on chat_ids / sharer_ids before we build the API request,
// so misuse surfaces as a named-flag validation error rather than an opaque
// server-side failure or empty result.
func validateDriveSearchIDs(spec driveSearchSpec) error {
	for _, id := range spec.CreatorIDs {
		if _, err := common.ValidateUserID(id); err != nil {
			return output.ErrValidation("--creator-ids %q: %s", id, err)
		}
	}
	if n := len(spec.ChatIDs); n > driveSearchMaxChatIDs {
		return output.ErrValidation("--chat-ids: max %d values per request, got %d", driveSearchMaxChatIDs, n)
	}
	for _, id := range spec.ChatIDs {
		if _, err := common.ValidateChatID(id); err != nil {
			return output.ErrValidation("--chat-ids %q: %s", id, err)
		}
	}
	if n := len(spec.SharerIDs); n > driveSearchMaxSharerIDs {
		return output.ErrValidation("--sharer-ids: max %d values per request, got %d", driveSearchMaxSharerIDs, n)
	}
	for _, id := range spec.SharerIDs {
		if _, err := common.ValidateUserID(id); err != nil {
			return output.ErrValidation("--sharer-ids %q: %s", id, err)
		}
	}
	return nil
}

func validateDocTypes(values []string) error {
	for _, v := range values {
		// values are already upper-cased by readDriveSearchSpec; compare as-is
		// so the filter we emit to the server matches what we validated.
		if _, ok := driveSearchDocTypeSet[v]; !ok {
			return output.ErrValidation("--doc-types contains unknown value %q (allowed: doc,sheet,bitable,mindnote,file,wiki,docx,folder,catalog,slides,shortcut)", v)
		}
	}
	return nil
}

// upperAll returns a copy of s with every element upper-cased.
func upperAll(s []string) []string {
	if len(s) == 0 {
		return s
	}
	out := make([]string, len(s))
	for i, v := range s {
		out[i] = strings.ToUpper(v)
	}
	return out
}

// clampOpenedTimeWindow enforces the server-side 3-month cap on open_time by
// narrowing --opened-since / --opened-until to the most recent slice and
// returning a notice that lists every remaining slice, so the agent can
// re-invoke for older ranges. When no clamping is needed, returns ("", nil).
//
// Rules:
//   - no --opened-since: skip (no range filter at all)
//   - only --opened-since or both set, span ≤ 90 days: skip
//   - span in (90, 365] days: clamp current request; spec is mutated in place
//     with RFC3339 values so buildTimeRangeFilter parses round-trip
//   - span > 365 days: validation error (prevents runaway slice counts)
func clampOpenedTimeWindow(spec *driveSearchSpec, now time.Time) (string, error) {
	if spec.OpenedSince == "" {
		return "", nil
	}
	sinceUnix, err := parseTimeValue(spec.OpenedSince, now)
	if err != nil {
		return "", output.ErrValidation("invalid --opened-since %q: %s", spec.OpenedSince, err)
	}
	var untilUnix int64
	if spec.OpenedUntil != "" {
		untilUnix, err = parseTimeValue(spec.OpenedUntil, now)
		if err != nil {
			return "", output.ErrValidation("invalid --opened-until %q: %s", spec.OpenedUntil, err)
		}
	} else {
		untilUnix = now.Unix()
	}
	if untilUnix <= sinceUnix {
		// Malformed range; let buildTimeRangeFilter / server surface the error.
		return "", nil
	}

	spanSecs := untilUnix - sinceUnix
	sliceSecs := int64(driveSearchSliceDays) * 24 * 3600
	if spanSecs <= sliceSecs {
		return "", nil
	}
	maxSecs := int64(driveSearchMaxOpenedSpanDays) * 24 * 3600
	if spanSecs > maxSecs {
		return "", output.ErrValidation(
			"--opened-* window spans %d days, exceeds the %d-day (1-year) maximum; narrow the range or run multiple queries",
			spanSecs/86400, driveSearchMaxOpenedSpanDays,
		)
	}

	// Build slices newest-to-oldest; last (oldest) slice may be shorter than 90d.
	numSlices := int((spanSecs + sliceSecs - 1) / sliceSecs) // ceil
	type sliceSpec struct{ start, end int64 }
	slices := make([]sliceSpec, numSlices)
	cursor := untilUnix
	for i := 0; i < numSlices; i++ {
		start := cursor - sliceSecs
		if start < sinceUnix {
			start = sinceUnix
		}
		slices[i] = sliceSpec{start: start, end: cursor}
		cursor = start
	}

	fmtTime := func(unix int64) string { return time.Unix(unix, 0).Format(time.RFC3339) }
	approxMonths := spanSecs / (30 * 24 * 3600)

	var b strings.Builder
	fmt.Fprintf(&b, "notice: --opened-* window spans %d days (~%d months), exceeds the server-side 3-month (%d-day) limit.\n",
		spanSecs/86400, approxMonths, driveSearchSliceDays)
	fmt.Fprintf(&b, "        this query was narrowed to the most recent slice; %d slices total:\n", numSlices)
	// Every slice — including the current one — prints concrete --opened-since
	// / --opened-until values so an agent paginating slice 1 can copy them
	// verbatim. Reusing the user's original relative time (e.g. "1y") would
	// re-resolve against time.Now() on the next call and silently drift the
	// window away from any --page-token issued for this call.
	for i, s := range slices {
		label := fmt.Sprintf("[slice %d/%d]", i+1, numSlices)
		if i == 0 {
			label = fmt.Sprintf("[slice %d/%d current]", i+1, numSlices)
		}
		// %-19s pads to "[slice N/M current]" (19 chars at the 5-slice cap).
		fmt.Fprintf(&b, "          %-19s --opened-since %s --opened-until %s\n",
			label, fmtTime(s.start), fmtTime(s.end))
	}
	fmt.Fprint(&b, "        pagination: paginate within a slice via --page-token using that slice's --opened-since / --opened-until values verbatim (NOT the original relative time like '1y' / '8m' — relative times re-resolve against time.Now() and would mismatch the page_token); switch to the next slice's --opened-* flags only after has_more=false, and do not carry --page-token across slices.")

	// Rewrite spec so buildTimeRangeFilter emits the clamped window.
	spec.OpenedSince = fmtTime(slices[0].start)
	spec.OpenedUntil = fmtTime(slices[0].end)

	return b.String(), nil
}

// buildTimeRangeFilter parses since/until for one dimension and applies hour
// snapping for server-aggregated fields. Returns nil range when both inputs
// are empty.
func buildTimeRangeFilter(key, since, until string, now time.Time) (map[string]interface{}, []string, error) {
	if since == "" && until == "" {
		return nil, nil, nil
	}
	_, hourAggregated := driveSearchHourAggregatedFields[key]

	rng := map[string]interface{}{}
	var notices []string

	if since != "" {
		unix, err := parseTimeValue(since, now)
		if err != nil {
			return nil, nil, output.ErrValidation("invalid --%s-since %q: %s", timeDimCLIName(key), since, err)
		}
		if hourAggregated && unix%3600 != 0 {
			snapped := floorHour(unix)
			notices = append(notices, formatHourSnapNotice(key, "start", unix, snapped))
			unix = snapped
		}
		rng["start"] = unix
	}
	if until != "" {
		unix, err := parseTimeValue(until, now)
		if err != nil {
			return nil, nil, output.ErrValidation("invalid --%s-until %q: %s", timeDimCLIName(key), until, err)
		}
		if hourAggregated && unix%3600 != 0 {
			snapped := ceilHour(unix)
			notices = append(notices, formatHourSnapNotice(key, "end", unix, snapped))
			unix = snapped
		}
		rng["end"] = unix
	}
	return rng, notices, nil
}

// timeDimCLIName maps a filter key back to the CLI flag prefix, for error
// messages that say "--edited-since" rather than "my_edit_time.start".
func timeDimCLIName(key string) string {
	switch key {
	case "my_edit_time":
		return "edited"
	case "my_comment_time":
		return "commented"
	case "open_time":
		return "opened"
	case "create_time":
		return "created"
	}
	return key
}

func formatHourSnapNotice(key, side string, before, after int64) string {
	return fmt.Sprintf("notice: %s has hour-level granularity server-side; %s %s → %s",
		key, side,
		time.Unix(before, 0).Format("2006-01-02 15:04:05"),
		time.Unix(after, 0).Format("2006-01-02 15:04:05"),
	)
}

func floorHour(unix int64) int64 {
	return unix - (unix % 3600)
}

func ceilHour(unix int64) int64 {
	if unix%3600 == 0 {
		return unix
	}
	return floorHour(unix) + 3600
}

var driveSearchRelativeRe = regexp.MustCompile(`^(\d+)([dmy])$`)

// parseTimeValue accepts relative (7d, 1m=30d, 1y=365d), absolute dates in a
// few common layouts, RFC3339, and raw unix seconds.
func parseTimeValue(input string, now time.Time) (int64, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return 0, fmt.Errorf("empty value")
	}

	if m := driveSearchRelativeRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		var days int
		switch m[2] {
		case "d":
			days = n
		case "m":
			days = n * 30
		case "y":
			days = n * 365
		}
		return now.Add(-time.Duration(days) * 24 * time.Hour).Unix(), nil
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t.Unix(), nil
		}
	}

	// Digit-only string at the end so "20260423" doesn't get misread as unix.
	// Real unix seconds for recent times are 10 digits; be conservative and
	// require length >= 10 to avoid matching YYYYMMDD. Mirror unixToISO8601's
	// ms-vs-s heuristic: 13-digit / >= 1e12 inputs are epoch-millis and get
	// normalized to seconds, otherwise a copy-pasted ms timestamp would
	// silently parse as a year-57000 unix and then trip the 1-year cap with
	// a misleading message.
	if len(s) >= 10 {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			if n >= 1e12 {
				n /= 1000
			}
			return n, nil
		}
	}

	return 0, fmt.Errorf("expected relative (7d/1m/1y), date (YYYY-MM-DD[ HH:MM:SS]), RFC3339, or unix seconds")
}

func callDriveSearchAPI(runtime *common.RuntimeContext, reqBody map[string]interface{}) (map[string]interface{}, error) {
	data, err := runtime.CallAPI("POST", "/open-apis/search/v2/doc_wiki/search", nil, reqBody)
	if err != nil {
		return nil, enrichDriveSearchError(err)
	}
	return data, nil
}

// enrichDriveSearchError adds a +search-specific hint for known opaque Lark
// codes; other errors pass through unchanged.
func enrichDriveSearchError(err error) error {
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		return err
	}
	if exitErr.Detail.Code != driveSearchErrUserNotVisible {
		return err
	}
	detail := *exitErr.Detail
	detail.Hint = "one or more open_ids in --creator-ids / --sharer-ids are outside this app's user-visibility scope (this is the app's contact visibility, not the search:docs:read API scope); ask an admin to grant the app visibility to those users in the developer console, or drop the unreachable open_ids"
	return &output.ExitError{
		Code:   exitErr.Code,
		Detail: &detail,
		Err:    exitErr.Err,
		Raw:    exitErr.Raw,
	}
}

func cloneDriveSearchFilter(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// renderDriveSearchTable mirrors the column layout of doc +search so the pretty
// output is consistent for users switching between the two.
func renderDriveSearchTable(w io.Writer, data map[string]interface{}, items []interface{}) {
	if len(items) == 0 {
		fmt.Fprintln(w, "No matching results found.")
		return
	}

	htmlTagRe := regexp.MustCompile(`</?hb?>`)
	var rows []map[string]interface{}
	for _, item := range items {
		u, _ := item.(map[string]interface{})
		if u == nil {
			continue
		}
		var rawTitle string
		if s, ok := u["title_highlighted"].(string); ok && s != "" {
			rawTitle = s
		} else if s, ok := u["title"].(string); ok {
			rawTitle = s
		}
		title := common.TruncateStr(htmlTagRe.ReplaceAllString(rawTitle, ""), 50)

		resultMeta, _ := u["result_meta"].(map[string]interface{})
		docTypes := ""
		if resultMeta != nil {
			docTypes = fmt.Sprintf("%v", resultMeta["doc_types"])
		}
		entityType := fmt.Sprintf("%v", u["entity_type"])
		typeStr := docTypes
		if typeStr == "" || typeStr == "<nil>" {
			typeStr = entityType
		}

		var url, editTime string
		if resultMeta != nil {
			if s, ok := resultMeta["url"].(string); ok {
				url = s
			}
			if s, ok := resultMeta["update_time_iso"].(string); ok {
				editTime = s
			}
		}
		if len(url) > 80 {
			url = url[:80]
		}

		rows = append(rows, map[string]interface{}{
			"type":      typeStr,
			"title":     title,
			"edit_time": editTime,
			"url":       url,
		})
	}

	output.PrintTable(w, rows)
	moreHint := ""
	hasMore, _ := data["has_more"].(bool)
	if hasMore {
		moreHint = " (more available, use --format json to get page_token, then --page-token to paginate)"
	}
	fmt.Fprintf(w, "\n%d result(s)%s\n", len(rows), moreHint)
}

// addDriveSearchIsoTimeFields recursively annotates every `*_time` numeric
// field with a matching `*_time_iso` RFC3339 string, so clients that parse
// JSON output don't have to convert epoch timestamps themselves.
func addDriveSearchIsoTimeFields(value interface{}) []interface{} {
	arr, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]interface{}, len(arr))
	for i, item := range arr {
		out[i] = addDriveSearchIsoTimeFieldsOne(item)
	}
	return out
}

func addDriveSearchIsoTimeFieldsOne(value interface{}) interface{} {
	switch v := value.(type) {
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = addDriveSearchIsoTimeFieldsOne(item)
		}
		return result
	case map[string]interface{}:
		out := make(map[string]interface{})
		for key, item := range v {
			if strings.HasSuffix(key, "_time_iso") {
				out[key] = item
				continue
			}
			out[key] = addDriveSearchIsoTimeFieldsOne(item)
			if strings.HasSuffix(key, "_time") {
				// If the input already carries the matching `_iso` sibling,
				// the iso-suffix passthrough branch will copy it; don't race
				// against it (map iteration order is non-deterministic).
				if _, exists := v[key+"_iso"]; exists {
					continue
				}
				if iso := unixToISO8601(item); iso != "" {
					out[key+"_iso"] = iso
				}
			}
		}
		return out
	default:
		return value
	}
}

func unixToISO8601(v interface{}) string {
	if v == nil {
		return ""
	}
	var num float64
	switch val := v.(type) {
	case float64:
		num = val
	case json.Number:
		parsed, err := val.Float64()
		if err != nil {
			return ""
		}
		num = parsed
	case string:
		parsed, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return ""
		}
		num = parsed
	case int64:
		num = float64(val)
	case int:
		num = float64(val)
	default:
		return ""
	}
	if math.IsInf(num, 0) || math.IsNaN(num) {
		return ""
	}
	secs := int64(num)
	if num >= 1e12 {
		secs = secs / 1000
	}
	return time.Unix(secs, 0).Format(time.RFC3339)
}
