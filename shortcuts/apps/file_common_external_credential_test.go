// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package apps

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/runtimeplan"
	"github.com/larksuite/cli/shortcuts/common"
)

type remoteFileRoundTripFunc func(*http.Request) (*http.Response, error)

func (f remoteFileRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type appsOpaqueFilePolicy struct{}

func (appsOpaqueFilePolicy) ValidateRemoteFile(rawURL string) error {
	if rawURL != "https://proxy.example/lark-cli/v1/files/opaque_1" {
		return errors.New("unexpected file handle")
	}
	return nil
}

func (appsOpaqueFilePolicy) UsesManagedFilePlane() bool { return true }

func TestProxyFileTransferClientDoesNotUseAPITimeout(t *testing.T) {
	base := &http.Client{
		Timeout: 30 * time.Second,
		Transport: remoteFileRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if _, hasDeadline := req.Context().Deadline(); hasDeadline {
				t.Fatal("managed file request inherited API client timeout")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("ok")),
				Request:    req,
			}, nil
		}),
	}
	factory := &cmdutil.Factory{HttpClient: func() (*http.Client, error) { return base, nil }}
	cmdutil.TestSetRuntimePlan(t, factory, runtimeplan.New(runtimeplan.Options{
		RemoteFiles: appsOpaqueFilePolicy{},
	}))
	runtime := &common.RuntimeContext{
		Config:  &core.CliConfig{},
		Factory: factory,
	}

	files := runtime.RemoteFiles()
	file, err := files.Validate(context.Background(), "https://proxy.example/lark-cli/v1/files/opaque_1")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, file.URL(), nil)
	resp, err := files.Do(req, file, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if base.Timeout != 30*time.Second {
		t.Fatalf("shared API client was mutated: timeout = %s", base.Timeout)
	}
}
