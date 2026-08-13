// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"errors"
	"testing"
)

func TestParseRef(t *testing.T) {
	r, err := ParseRef("example:agt_xxx")
	if err != nil || r.Scheme != "example" || r.AgentID != "agt_xxx" {
		t.Fatalf("got %+v err=%v", r, err)
	}
}

func TestParseRefErrors(t *testing.T) {
	for _, s := range []string{"", "example", "example:", ":agt", "example:agt:extra"} {
		if _, err := ParseRef(s); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("ParseRef(%q) should return ErrInvalidRef, got err=%v", s, err)
		}
	}
}
