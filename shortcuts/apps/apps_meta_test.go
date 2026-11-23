// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func newMetaTestRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_meta_test"}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	rt := common.TestNewRuntimeContextForAPI(
		context.Background(),
		&cobra.Command{Use: "+meta-test"},
		cfg, f, core.AsUser,
	)
	return rt, reg
}

func TestQueryAppType_Success(t *testing.T) {
	rt, reg := newMetaTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_test",
		Body: map[string]interface{}{
			"code": float64(0),
			"data": map[string]interface{}{
				"app": map[string]interface{}{
					"app_id":   "app_test",
					"app_type": "MODERN_HTML",
				},
			},
		},
	})

	result, err := queryAppType(context.Background(), rt, "app_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "modern_html" {
		t.Errorf("queryAppType = %q, want modern_html", result)
	}
}

func TestQueryAppType_FullStack(t *testing.T) {
	rt, reg := newMetaTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_fs",
		Body: map[string]interface{}{
			"code": float64(0),
			"data": map[string]interface{}{
				"app": map[string]interface{}{
					"app_id":   "app_fs",
					"app_type": "FULL_STACK",
				},
			},
		},
	})

	result, err := queryAppType(context.Background(), rt, "app_fs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "full_stack" {
		t.Errorf("queryAppType = %q, want full_stack", result)
	}
}

func TestQueryAppType_Html(t *testing.T) {
	rt, reg := newMetaTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_html",
		Body: map[string]interface{}{
			"code": float64(0),
			"data": map[string]interface{}{
				"app": map[string]interface{}{
					"app_id":   "app_html",
					"app_type": "HTML",
				},
			},
		},
	})

	result, err := queryAppType(context.Background(), rt, "app_html")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "html" {
		t.Errorf("queryAppType = %q, want html", result)
	}
}

func TestQueryAppType_APIError(t *testing.T) {
	rt, reg := newMetaTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_bad",
		Status: 500,
		Body:   map[string]interface{}{"code": float64(99999), "msg": "internal error"},
	})

	_, err := queryAppType(context.Background(), rt, "app_bad")
	if err == nil {
		t.Error("expected error on API failure")
	}
}

func TestQueryAppType_MissingAppObject(t *testing.T) {
	rt, reg := newMetaTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_no",
		Body: map[string]interface{}{
			"code": float64(0),
			"data": map[string]interface{}{},
		},
	})

	_, err := queryAppType(context.Background(), rt, "app_no")
	if err == nil {
		t.Error("expected error when app object missing")
	}
}

func TestQueryAppType_EmptyAppType(t *testing.T) {
	rt, reg := newMetaTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_empty",
		Body: map[string]interface{}{
			"code": float64(0),
			"data": map[string]interface{}{
				"app": map[string]interface{}{
					"app_id":   "app_empty",
					"app_type": "",
				},
			},
		},
	})

	_, err := queryAppType(context.Background(), rt, "app_empty")
	if err == nil {
		t.Error("expected error when app_type is empty")
	}
}
