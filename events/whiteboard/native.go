// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

// BoardWhiteboardUpdatedV1Data is the flattened whiteboard updated source payload.
type BoardWhiteboardUpdatedV1Data struct {
	// WhiteboardID is the id of the whiteboard whose content was updated.
	WhiteboardID string `json:"whiteboard_id"`
	// OperatorIDs lists the operators that produced this update batch.
	OperatorIDs []OperatorID `json:"operator_ids"`
}

// OperatorID identifies an operator that produced the whiteboard update,
// expressed in the three Lark identity formats.
type OperatorID struct {
	// OpenID is the operator's open_id within the current app.
	OpenID string `json:"open_id"`
	// UnionID is the operator's union_id across apps under the same ISV.
	UnionID string `json:"union_id"`
	// UserID is the operator's user_id within the tenant.
	UserID string `json:"user_id"`
}
