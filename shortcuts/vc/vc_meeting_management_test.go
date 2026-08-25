// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"net/http"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

type meetingManagementErrorTokenResolver struct {
	err error
}

func (r *meetingManagementErrorTokenResolver) ResolveToken(context.Context, credential.TokenSpec) (*credential.TokenResult, error) {
	return nil, r.err
}

func TestCallMeetingManagementAPIEnvelopePreservesTypedDoAPIErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "authentication",
			err:  errs.NewAuthenticationError(errs.SubtypeTokenExpired, "token expired"),
		},
		{
			name: "network",
			err:  errs.NewNetworkError(errs.SubtypeNetworkTransport, "network unavailable"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultConfig()
			f, _, _, _ := cmdutil.TestFactory(t, cfg)
			f.Credential = credential.NewCredentialProvider(nil, nil, &meetingManagementErrorTokenResolver{err: tt.err}, nil)
			runtime := common.TestNewRuntimeContextForAPI(
				context.Background(),
				&cobra.Command{Use: "+meeting-end"},
				cfg,
				f,
				core.AsUser,
			)

			envelope, data, gotErr := callMeetingManagementAPIEnvelope(
				runtime,
				http.MethodPatch,
				"/open-apis/vc/v1/meetings/7651377260537433044/end",
				nil,
			)
			if gotErr != tt.err {
				t.Fatalf("error = %T %v, want original %T %v", gotErr, gotErr, tt.err, tt.err)
			}
			if envelope != nil || data != nil {
				t.Fatalf("envelope, data = %#v, %#v, want nil, nil", envelope, data)
			}
		})
	}
}

func TestCallMeetingManagementAPIEnvelopeInjectsHeaderLogIDWithoutDroppingServerFields(t *testing.T) {
	cfg := defaultConfig()
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	reg.Register(&httpmock.Stub{
		Method: http.MethodPatch,
		URL:    "/open-apis/vc/v1/meetings/7651377260537433044/end",
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Tt-Logid":   []string{"header-log-id"},
		},
		Body: map[string]any{
			"code":        0,
			"msg":         "ok",
			"server_meta": "preserved",
			"data": map[string]any{
				"ended":         true,
				"server_detail": "preserved",
			},
		},
	})
	runtime := common.TestNewRuntimeContextForAPI(
		context.Background(),
		&cobra.Command{Use: "+meeting-end"},
		cfg,
		f,
		core.AsUser,
	)

	envelope, data, err := callMeetingManagementAPIEnvelope(
		runtime,
		http.MethodPatch,
		"/open-apis/vc/v1/meetings/7651377260537433044/end",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if envelope["log_id"] != "header-log-id" {
		t.Fatalf("log_id = %#v, want header-log-id", envelope["log_id"])
	}
	if envelope["server_meta"] != "preserved" {
		t.Fatalf("server_meta = %#v, want preserved", envelope["server_meta"])
	}
	if envelopeData, ok := envelope["data"].(map[string]any); !ok {
		t.Fatalf("envelope data = %T %#v, want object", envelope["data"], envelope["data"])
	} else if envelopeData["ended"] != true || envelopeData["server_detail"] != "preserved" {
		t.Fatalf("envelope data = %#v, want all server fields preserved", envelopeData)
	}
	if data["ended"] != true || data["server_detail"] != "preserved" {
		t.Fatalf("classified data = %#v, want all server fields preserved", data)
	}
}
