// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// INTERNAL-ONLY: DO NOT CHERRY-PICK TO github.com/larksuite/cli
//
// This file, and the three `&& !isSandboxProxyIP(...)` clauses it serves in
// url.go, exist only for the Doubao/TAE cloud sandbox. They encode that
// sandbox's own proxy addresses, so they are meaningless outside it and must not
// reach the public repository — not as part of a sync, a squash-merge, or a
// cherry-pick of a commit that happens to touch url.go.
//
// Why it must stay internal, beyond it being someone else's address: on a
// public user's machine 127.0.0.1-127.0.0.5 are their own loopback addresses,
// where unauthenticated local services live. Shipping these exemptions hands
// every downstream user a weaker SSRF guard for no benefit they can use.
//
// If the public CLI ever needs this, it needs a different design: a declared
// egress-delegation contract rather than one vendor's proxy IPs.

package validate

import (
	"net"
)

var sandboxProxyIPs = [...]net.IP{
	net.IPv4(127, 0, 0, 1),
	net.IPv4(127, 0, 0, 2),
	net.IPv4(127, 0, 0, 3),
	net.IPv4(127, 0, 0, 4),
	net.IPv4(127, 0, 0, 5),
}

// isSandboxProxyIP reports whether ip is one of the explicitly configured
// Doubao sandbox proxy addresses.
//
// Callers apply it only to a *resolved* address. Literal 127.0.0.1-127.0.0.5
// URLs are rejected earlier, before any resolution runs, as are 10.0.0.0/8 and
// 169.254.169.254 — the branch every practical SSRF payload takes.
func isSandboxProxyIP(ip net.IP) bool {
	for _, proxyIP := range sandboxProxyIPs {
		if ip.Equal(proxyIP) {
			return true
		}
	}
	return false
}
