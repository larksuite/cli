// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"net/http"
	"testing"

	exttransport "github.com/larksuite/cli/extension/transport"
)

type appsExternalProvider struct {
	interceptor exttransport.Interceptor
}

func (p appsExternalProvider) Name() string { return "apps-external-test" }

func (p appsExternalProvider) ResolveInterceptor(context.Context) exttransport.Interceptor {
	return p.interceptor
}

func (appsExternalProvider) SupportsRequestClass(class exttransport.RequestClass) bool {
	return class == exttransport.RequestClassExternal
}

type appsExternalInterceptor struct {
	calls int
}

func (i *appsExternalInterceptor) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	i.calls++
	req.Header.Set("X-External-Route", "1")
	return nil
}

type appsRoundTripFunc func(*http.Request) (*http.Response, error)

func (f appsRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFileTransferClientUsesExternalRequestClass(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARK_CLI_NO_PROXY", "")

	previousProvider := exttransport.GetProvider()
	interceptor := &appsExternalInterceptor{}
	exttransport.Register(appsExternalProvider{interceptor: interceptor})
	t.Cleanup(func() { exttransport.Register(previousProvider) })

	previousTransport := http.DefaultTransport
	var receivedHeader string
	http.DefaultTransport = appsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		receivedHeader = req.Header.Get("X-External-Route")
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     make(http.Header),
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	req, err := http.NewRequest(http.MethodGet, "https://open.feishu.cn/presigned/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := newFileTransferClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if interceptor.calls != 1 || receivedHeader != "1" {
		t.Fatalf("external route = calls %d, header %q; want 1, %q", interceptor.calls, receivedHeader, "1")
	}
}
