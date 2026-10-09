// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

func TestSynchronizeClockValidatesResponseBeforeUpdating(t *testing.T) {
	for _, brand := range []core.LarkBrand{core.BrandFeishu, core.BrandLark} {
		for _, tc := range []struct {
			name   string
			status int
			body   string
			valid  bool
		}{
			{"success", 200, fmt.Sprintf(`{"code":0,"data":{"now":"%d"}}`, time.Now().Add(time.Hour).Unix()), true},
			{"http_error", 503, `{"code":0,"data":{"now":"1700000000"}}`, false},
			{"business_error", 200, `{"code":1,"data":{"now":"1700000000"}}`, false},
			{"invalid_json", 200, `{`, false}, {"missing_time", 200, `{"code":0}`, false},
			{"numeric_time", 200, `{"code":0,"data":{"now":1700000000}}`, false},
			{"negative_time", 200, `{"code":0,"data":{"now":"-1"}}`, false},
			{"overflow", 200, `{"code":0,"data":{"now":"99999999999999999999"}}`, false},
		} {
			t.Run(string(brand)+"/"+tc.name, func(t *testing.T) {
				binding, _ := transportTestBinding(t)
				key := binding.Key()
				prior := ClockState{OffsetMillis: -3600000, SyncedAtMillis: 1700000000000}
				key.Clock().RestoreState(prior)
				body := &transportTestBody{Reader: strings.NewReader(tc.body)}
				client := &http.Client{Transport: transportTestRoundTripper(func(req *http.Request) (*http.Response, error) {
					if req.Method != http.MethodPost || req.URL.String() != core.ResolveEndpoints(brand).Accounts+HeartbeatPath || req.Header.Get("Authorization") != "" || req.Header.Get(ProofHeader) != "" {
						t.Errorf("unexpected heartbeat request: %s %s", req.Method, req.URL)
					}
					return &http.Response{StatusCode: tc.status, Header: http.Header{}, Body: body, Request: req}, nil
				})}
				err := SynchronizeClock(context.Background(), client, brand, key)
				if body.closes != 1 {
					t.Fatal("heartbeat response body leaked")
				}
				if tc.valid {
					if err != nil {
						t.Fatal(err)
					}
					offset := key.Clock().State().OffsetMillis
					if offset < 3598000 || offset > 3601000 || key.Clock().State().SyncedAtMillis <= prior.SyncedAtMillis {
						t.Fatal("clock did not use server time")
					}
				} else {
					p, ok := errs.ProblemOf(err)
					if !ok || p.Subtype != errs.SubtypeDPoPClockSyncFailed || p.Hint == "" || key.Clock().State() != prior {
						t.Fatalf("invalid response changed clock or lost typed error: %v", err)
					}
				}
			})
		}
	}
}

func TestSynchronizeClockPreservesTransportAndReadErrors(t *testing.T) {
	for _, readFailure := range []bool{false, true} {
		cause := errors.New("injected heartbeat failure")
		binding, _ := transportTestBinding(t)
		client := &http.Client{Transport: transportTestRoundTripper(func(req *http.Request) (*http.Response, error) {
			if !readFailure {
				return nil, cause
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(errorReader{cause}), Request: req}, nil
		})}
		if err := SynchronizeClock(context.Background(), client, core.BrandFeishu, binding.Key()); !errors.Is(err, cause) {
			t.Fatalf("lost underlying cause: %v", err)
		}
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
