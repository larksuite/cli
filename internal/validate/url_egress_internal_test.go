// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// INTERNAL-ONLY: DO NOT CHERRY-PICK TO github.com/larksuite/cli — see
// url_egress.go for why the exemption under test must stay internal.

package validate

import (
	"context"
	"net"
	"net/http"
	"testing"
)

func TestControlledEgress(t *testing.T) {
	cases := []struct {
		name     string
		host     string // hostname, or a literal IP to exercise that branch
		resolves string // ignored when host is a literal IP
		wantOK   bool
	}{
		{"hostname to sandbox proxy 1", "download.example", "127.0.0.1", true},
		{"hostname to sandbox proxy 2", "download.example", "127.0.0.2", true},
		{"hostname to sandbox proxy 3", "download.example", "127.0.0.3", true},
		{"hostname to sandbox proxy 4", "download.example", "127.0.0.4", true},
		{"hostname to sandbox proxy 5", "download.example", "127.0.0.5", true},

		// Literal IPs never benefit: this is the branch every practical SSRF
		// payload takes.
		{"literal sandbox proxy IP 1", "127.0.0.1", "", false},
		{"literal sandbox proxy IP 5", "127.0.0.5", "", false},
		{"literal cloud metadata", "169.254.169.254", "", false},
		{"literal rfc1918", "10.1.2.3", "", false},
		{"localhost", "localhost", "", false},

		// Every unlisted resolved address keeps its original verdict.
		{"hostname to unlisted loopback", "evil.example", "127.0.0.6", false},
		{"hostname to rfc1918", "evil.example", "10.1.2.3", false},
		{"hostname to metadata", "evil.example", "169.254.169.254", false},
		{"hostname to CGNAT", "cgnat.example", "100.64.0.5", false},

		// Public addresses are unaffected either way.
		{"public host", "public.example", "93.184.216.34", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(context.Context, string, string) ([]net.IP, error) {
				return []net.IP{net.ParseIP(tc.resolves)}, nil
			}
			_, err := resolveDownloadHost(context.Background(), tc.host, lookup)
			if gotOK := err == nil; gotOK != tc.wantOK {
				t.Fatalf("host %q -> %q: allowed=%v want=%v (err=%v)", tc.host, tc.resolves, gotOK, tc.wantOK, err)
			}
		})
	}
}

// The exemption lives in three places and each must be asserted at its own
// entry point: a test that only exercises isSandboxProxyIP still passes if
// either call site is reverted, which is exactly the regression worth catching.
//
// resolveDownloadHost is covered by TestControlledEgress above.

func TestControlledEgressPinTarget(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://download.example/file", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	for _, tc := range []struct {
		name   string
		ip     string
		wantOK bool
	}{
		{"sandbox proxy 1", "127.0.0.1", true},
		{"sandbox proxy 2", "127.0.0.2", true},
		{"sandbox proxy 3", "127.0.0.3", true},
		{"sandbox proxy 4", "127.0.0.4", true},
		{"sandbox proxy 5", "127.0.0.5", true},
		{"other loopback", "127.0.0.6", false},
		{"rfc1918", "10.1.2.3", false},
		{"metadata", "169.254.169.254", false},
		{"public", "93.184.216.34", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pinned, err := pinDownloadRequestTargetToIP(req, net.ParseIP(tc.ip))
			if gotOK := err == nil; gotOK != tc.wantOK {
				t.Fatalf("pin %s: allowed=%v want=%v (err=%v)", tc.ip, gotOK, tc.wantOK, err)
			}
			if err == nil && pinned.Host != "download.example" {
				t.Errorf("pinned.Host = %q, want the original host preserved", pinned.Host)
			}
		})
	}
}

func TestControlledEgressConnGuard(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ip     string
		wantOK bool
	}{
		{"sandbox proxy 1", "127.0.0.1", true},
		{"sandbox proxy 2", "127.0.0.2", true},
		{"sandbox proxy 3", "127.0.0.3", true},
		{"sandbox proxy 4", "127.0.0.4", true},
		{"sandbox proxy 5", "127.0.0.5", true},
		{"other loopback", "127.0.0.6", false},
		{"rfc1918", "10.1.2.3", false},
		{"metadata", "169.254.169.254", false},
		{"cgnat", "100.64.0.5", false},
		{"public", "93.184.216.34", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := &trackedDownloadConn{
				remoteAddr: &net.TCPAddr{IP: net.ParseIP(tc.ip), Port: 443},
			}
			err := validateConnRemoteIP(conn)
			if gotOK := err == nil; gotOK != tc.wantOK {
				t.Fatalf("conn from %s: allowed=%v want=%v (err=%v)", tc.ip, gotOK, tc.wantOK, err)
			}
		})
	}
}
