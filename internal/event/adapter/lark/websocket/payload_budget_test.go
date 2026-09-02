// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package websocket

import (
	"testing"

	"github.com/larksuite/cli/internal/event/adapter/localbus/protocol"
)

// The ingress cap and the local wire format's payload budget have to agree.
// They are declared separately on purpose — the platform ingress does not
// depend on how events are framed locally, so swapping the local transport
// leaves it untouched — which means nothing but this test keeps them in step.
//
// When they drifted apart, the ingress accepted payloads the bus could frame
// but the consumer refused to read, and the event was lost with a warning.
func TestPayloadBudget_MatchesTheWireFormat(t *testing.T) {
	if maxEventBodyBytes > protocol.MaxEventPayloadBytes {
		t.Errorf("the ingress accepts up to %d bytes but the wire format promises only %d; every event in between is accepted and then dropped",
			maxEventBodyBytes, protocol.MaxEventPayloadBytes)
	}
	if maxEventBodyBytes < protocol.MaxEventPayloadBytes {
		t.Errorf("the ingress accepts only %d bytes while the wire format allows %d; the difference is capacity thrown away silently",
			maxEventBodyBytes, protocol.MaxEventPayloadBytes)
	}
}
