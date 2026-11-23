// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package render turns consume decisions into user-facing output. It is the
// only place a decision becomes JSON; the application layer never formats
// anything itself.
package render

import (
	"io"
	"regexp"

	appconsume "github.com/larksuite/cli/internal/event/application/consume"
	"github.com/larksuite/cli/internal/output"
)

// sensitiveParamName matches parameter names whose values must never be
// echoed back in a rendered decision. Names are matched, not values: a
// credential-bearing parameter is identifiable by its declaration, and
// guessing at value shapes would miss more than it catches.
var sensitiveParamName = regexp.MustCompile(`(?i)(token|secret|password|credential|cookie)`)

func redactParams(params map[string]string) map[string]string {
	out := make(map[string]string, len(params))
	for name, value := range params {
		if sensitiveParamName.MatchString(name) {
			out[name] = "[redacted]"
			continue
		}
		out[name] = value
	}
	return out
}

// decisionPayload is the JSON shape under data.decision — snake_case, stable,
// documented in the event skill. Field additions must be additive.
type decisionPayload struct {
	EventKey      string             `json:"event_key"`
	Domain        string             `json:"domain"`
	Identity      string             `json:"identity"`
	Status        string             `json:"status"`
	Params        map[string]string  `json:"params"`
	Scope         string             `json:"scope"`
	Preconditions []preconditionView `json:"preconditions"`
	Preparation   *preparationView   `json:"preparation,omitempty"`
	WouldRead     []string           `json:"would_read"`
	WouldWrite    []string           `json:"would_write"`
}

type preconditionView struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	// The machine-readable half of a failure, mirroring the error envelope a
	// real run would emit: callers branch on subtype and act on hint instead of
	// matching prose. Omitted when the check did not fail.
	Subtype       string   `json:"subtype,omitempty"`
	Hint          string   `json:"hint,omitempty"`
	MissingScopes []string `json:"missing_scopes,omitempty"`
}

type preparationView struct {
	Strategy  string `json:"strategy"`
	Condition string `json:"condition"`
	Action    string `json:"action"`
}

// WriteDecisionJSON emits the decision inside the standard success envelope
// with the envelope's own top-level dry_run marker set.
func WriteDecisionJSON(out, errOut io.Writer, identity string, v appconsume.DecisionView) error {
	return output.WriteSuccessEnvelope(map[string]any{
		"decision": toPayload(v),
	}, output.SuccessEnvelopeOptions{
		CommandPath: "event consume",
		Identity:    identity,
		DryRun:      true,
		Out:         out,
		ErrOut:      errOut,
	})
}

func toPayload(v appconsume.DecisionView) decisionPayload {
	p := decisionPayload{
		EventKey:   v.EventKey,
		Domain:     v.Domain,
		Identity:   v.Identity,
		Status:     v.Status,
		Params:     redactParams(v.Params),
		Scope:      v.Scope,
		WouldRead:  v.WouldRead,
		WouldWrite: v.WouldWrite,
	}
	p.Preconditions = make([]preconditionView, 0, len(v.Preconditions))
	for _, pc := range v.Preconditions {
		p.Preconditions = append(p.Preconditions, preconditionView(pc))
	}
	if v.Preparation != nil {
		pv := preparationView(*v.Preparation)
		p.Preparation = &pv
	}
	return p
}
