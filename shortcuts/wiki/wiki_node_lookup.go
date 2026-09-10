// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// lookupWikiNode resolves either a Wiki node_token or a document obj_token.
func lookupWikiNode(runtime *common.RuntimeContext, token string) (*wikiNodeRecord, error) {
	data, err := runtime.CallAPITyped("GET", "/open-apis/wiki/v2/spaces/node_by_token", map[string]interface{}{"token": token}, nil)
	if err != nil {
		return nil, wikiNodeLookupProblem(err)
	}
	return parseWikiNodeRecord(common.GetMap(data, "node"))
}

// These business codes belong to node_by_token regardless of the caller.
// Command-specific recovery hints stay with the command.
func wikiNodeLookupProblem(err error) error {
	if p, ok := errs.ProblemOf(err); ok {
		switch p.Code {
		case 131012:
			p.Subtype, p.Retryable = errs.SubtypeNotFound, false
		case 131013, 131016:
			p.Subtype, p.Retryable = errs.SubtypeInvalidParameters, false
		case 131014:
			p.Subtype, p.Retryable = errs.SubtypeFailedPrecondition, false
		}
	}
	return err
}
