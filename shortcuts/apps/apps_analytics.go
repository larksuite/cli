// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const (
	defaultAppsAnalyticsEnv      = "online"
	defaultAppsAnalyticsGranular = "day"
	analyticsListEndpoint        = "query_analytics_data"
)

// AppsAnalyticsList lists online app product analytics.
var AppsAnalyticsList = common.Shortcut{
	Service:     appsService,
	Command:     "+analytics-list",
	Description: "List online app user and page-view analytics",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +analytics-list --app-id <app_id> --analytics users --granularity week",
		"Tip: analytics timestamps use nanoseconds; use +metric-list for request/runtime metrics.",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID whose online analytics should be listed", Required: true},
		{Name: appsEnvironmentFlag, Default: defaultAppsAnalyticsEnv, Desc: "observability environment; only online is supported"},
		{Name: "analytics", Desc: "analytics family to list", Required: true, Enum: []string{"users", "page-view"}},
		{Name: "series", Desc: "analytics series within the family, such as active-users or desktop-view"},
		{Name: "since", Desc: "start time, relative duration (30s, 5m, 0.5h, 2h, 3d, 1w), local date/time, or RFC3339; defaults to 30 days before --until"},
		{Name: "until", Desc: "end time, relative duration (30s, 5m, 0.5h, 2h, 3d, 1w), local date/time, or RFC3339; defaults to now"},
		{Name: "page", Desc: "frontend page or route filter"},
		{Name: "device-type", Desc: "device type filter", Enum: []string{"desktop", "mobile"}},
		{Name: "granularity", Default: defaultAppsAnalyticsGranular, Desc: "analytics aggregation granularity", Enum: []string{"day", "week", "month"}},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		_, _, _, err := buildAnalyticsListBody(rctx)
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		body, _, _, _ := buildAnalyticsListBody(rctx)
		return common.NewDryRunAPI().
			POST(analyticsListPath(rctx.Str("app-id"))).
			Desc("List online app analytics").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, _ := requireAppID(rctx.Str("app-id"))
		body, types, labels, err := buildAnalyticsListBody(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", analyticsListPath(appID), nil, body)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		out := observabilitySeriesOutput{
			Items:   normalizeAnalyticsSeries(data, types, labels),
			HasMore: false,
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			rows := observabilitySeriesRows(out.Items)
			sortObservabilityRowsDesc(rows, "timestamp_ns")
			rows = filterObservabilityRowsWithTime(rows, "timestamp_ns")
			appsPrintSchemaTable(w, rows, analyticsSeriesSchema(labels))
		})
		return nil
	},
}

func analyticsListPath(appID string) string {
	return appScopedPath(appID, analyticsListEndpoint)
}

func buildAnalyticsListBody(rctx *common.RuntimeContext) (map[string]interface{}, []string, []string, error) {
	env := strings.TrimSpace(rctx.Str(appsEnvironmentFlag))
	if env == "" {
		env = defaultAppsAnalyticsEnv
	}
	if err := validateObservabilityEnv(env); err != nil {
		return nil, nil, nil, err
	}
	types, labels, filter, err := analyticsTypesForCLI(rctx.Str("analytics"), rctx.Str("series"), rctx.Str("device-type"))
	if err != nil {
		return nil, nil, nil, err
	}
	since, until, err := defaultedObservabilityTimeRange(rctx.Str("since"), rctx.Str("until"))
	if err != nil {
		return nil, nil, nil, err
	}
	aggregation, err := analyticsGranularityForCLI(rctx.Str("granularity"))
	if err != nil {
		return nil, nil, nil, err
	}
	if page := strings.TrimSpace(rctx.Str("page")); page != "" {
		filter["page"] = page
	}
	body := map[string]interface{}{
		"metric_types":          types,
		"start_timestamp_ns":    nsNumber(since),
		"end_timestamp_ns":      nsNumber(until),
		"time_aggregation_unit": aggregation,
		"need_pack_lack_point":  false,
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}
	return body, types, labels, nil
}

func analyticsTypesForCLI(name, series, deviceType string) ([]string, []string, map[string]interface{}, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	series = strings.TrimSpace(strings.ToLower(series))
	deviceType = strings.TrimSpace(strings.ToLower(deviceType))
	filter := make(map[string]interface{})
	if deviceType != "" {
		switch deviceType {
		case "desktop", "mobile":
			filter["device_types"] = []string{deviceType}
		default:
			return nil, nil, nil, appsValidationParamError("--device-type", "--device-type must be desktop or mobile")
		}
	}

	switch name {
	case "users":
		switch series {
		case "":
			return []string{"ACTIVE_USER", "NEW_USER", "TOTAL_USER"}, []string{"active-users", "new-users", "total-users"}, filter, nil
		case "active", "active-users":
			return []string{"ACTIVE_USER"}, []string{"active-users"}, filter, nil
		case "new", "new-users":
			return []string{"NEW_USER"}, []string{"new-users"}, filter, nil
		case "total", "total-users":
			return []string{"TOTAL_USER"}, []string{"total-users"}, filter, nil
		default:
			return nil, nil, nil, appsValidationParamError("--series", "--series for --analytics users must be active, new, or total")
		}
	case "page-view":
		switch series {
		case "", "all":
			return []string{"PAGE_VIEW"}, []string{"all"}, filter, nil
		case "desktop", "desktop-view":
			if err := mergeAnalyticsDeviceFilter(filter, "desktop"); err != nil {
				return nil, nil, nil, err
			}
			return []string{"PAGE_VIEW"}, []string{"desktop"}, filter, nil
		case "mobile", "mobile-view":
			if err := mergeAnalyticsDeviceFilter(filter, "mobile"); err != nil {
				return nil, nil, nil, err
			}
			return []string{"PAGE_VIEW"}, []string{"mobile"}, filter, nil
		default:
			return nil, nil, nil, appsValidationParamError("--series", "--series for --analytics page-view must be all, desktop, or mobile")
		}
	default:
		return nil, nil, nil, appsValidationParamError("--analytics", "--analytics must be users or page-view")
	}
}

func mergeAnalyticsDeviceFilter(filter map[string]interface{}, deviceType string) error {
	if existing, ok := filter["device_types"].([]string); ok && len(existing) > 0 && existing[0] != deviceType {
		return appsValidationParamError("--device-type", "--device-type conflicts with --series")
	}
	filter["device_types"] = []string{deviceType}
	return nil
}

func analyticsGranularityForCLI(granularity string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(granularity)) {
	case "", "day":
		return "DAY", nil
	case "week":
		return "WEEK", nil
	case "month":
		return "MONTH", nil
	default:
		return "", appsValidationParamError("--granularity", "--granularity must be day, week, or month")
	}
}

func normalizeAnalyticsSeries(data map[string]interface{}, names, labels []string) []map[string]interface{} {
	items := normalizeObservabilitySeries(data, labels, observabilityNameLabels(names, labels), false, "timestamp_ns")
	fillObservabilityZeroesWhenPartiallyPresent(items, labels)
	return items
}

func analyticsSeriesSchema(labels []string) appsOutputSchema {
	columns := []appsOutputColumn{
		{Key: "timestamp_ns", Label: "time", Format: appsFormatNS("2006-01-02 15:04:05")},
	}
	for _, label := range labels {
		columns = append(columns, appsOutputColumn{Key: label})
	}
	return appsOutputSchema{Columns: columns, Strict: true}
}
