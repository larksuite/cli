// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestDoStream_HTTPErrorIncludesLogID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	config := &core.CliConfig{AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu}
	factory, _, _, reg := cmdutil.TestFactory(t, config)
	reg.Register(&httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "/open-apis/drive/v1/medias/file_token/download",
		Status:  http.StatusForbidden,
		RawBody: []byte("forbidden"),
		Headers: http.Header{
			larkcore.HttpHeaderKeyLogId: []string{"202605270003"},
		},
	})

	client, err := factory.NewAPIClientWithConfig(config)
	if err != nil {
		t.Fatalf("NewAPIClientWithConfig() error = %v", err)
	}

	_, err = client.DoStream(context.Background(), &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    "/open-apis/drive/v1/medias/file_token/download",
	}, core.AsBot)
	var netErr *errs.NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("expected *errs.NetworkError, got %T %v", err, err)
	}
	if netErr.LogID != "202605270003" {
		t.Fatalf("LogID = %q, want %q", netErr.LogID, "202605270003")
	}
}
