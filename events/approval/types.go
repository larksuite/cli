// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package approval

// ApprovalUserID identifies a user in the three Lark ID formats included by
// approval status-change events.
type ApprovalUserID struct {
	OpenID  string `json:"open_id,omitempty"  desc:"User open_id; prefixed with ou_" kind:"open_id"`
	UnionID string `json:"union_id,omitempty" desc:"User union_id"                  kind:"union_id"`
	UserID  string `json:"user_id,omitempty"  desc:"User id within the tenant"       kind:"user_id"`
}

// ApprovalInstanceStatusChangedV4Output is the flattened shape for
// approval.instance.status_changed_v4.
type ApprovalInstanceStatusChangedV4Output struct {
	Type         string          `json:"type"                    desc:"Event type; always approval.instance.status_changed_v4" enum:"approval.instance.status_changed_v4"`
	EventID      string          `json:"event_id,omitempty"      desc:"Globally unique event ID; safe for deduplication"`
	Timestamp    string          `json:"timestamp,omitempty"     desc:"Event delivery time (ms timestamp string); taken from header.create_time when present" kind:"timestamp_ms"`
	ApprovalCode string          `json:"approval_code,omitempty" desc:"Approval definition code; not a subscription dimension"`
	InstanceCode string          `json:"instance_code,omitempty" desc:"Approval instance code"`
	ExternalID   string          `json:"external_id,omitempty"   desc:"Third-party approval instance id; present only for third-party approvals"`
	Status       string          `json:"status,omitempty"        desc:"Approval instance status" enum:"PENDING,APPROVED,REJECTED,CANCELED,DELETED,REVERTED,OVERTIME_CLOSE,OVERTIME_RECOVER"`
	OperateTime  string          `json:"operate_time,omitempty"  desc:"Status change time in milliseconds" kind:"timestamp_ms"`
	StartUser    *ApprovalUserID `json:"start_user,omitempty"    desc:"Approval instance starter; omitted when unavailable"`
}

// ApprovalTaskStatusChangedV4Output is the flattened shape for
// approval.task.status_changed_v4.
type ApprovalTaskStatusChangedV4Output struct {
	Type           string          `json:"type"                      desc:"Event type; always approval.task.status_changed_v4" enum:"approval.task.status_changed_v4"`
	EventID        string          `json:"event_id,omitempty"        desc:"Globally unique event ID; safe for deduplication"`
	Timestamp      string          `json:"timestamp,omitempty"       desc:"Event delivery time (ms timestamp string); taken from header.create_time when present" kind:"timestamp_ms"`
	ApprovalCode   string          `json:"approval_code,omitempty"     desc:"Approval definition code; not a subscription dimension"`
	InstanceCode   string          `json:"instance_code,omitempty"     desc:"Approval instance code"`
	TaskID         string          `json:"task_id,omitempty"           desc:"Approval task id"`
	ExternalID     string          `json:"external_id,omitempty"       desc:"Third-party approval external id; present only for third-party approvals"`
	TaskExternalID string          `json:"task_external_id,omitempty"  desc:"Third-party approval task external id; present only when emitted by the upstream service"`
	AssignedUser   *ApprovalUserID `json:"assigned_user,omitempty"     desc:"Task assignee or operator user ids; omitted for automatic flows without an operator"`
	Status         string          `json:"status,omitempty"            desc:"Approval task status" enum:"REVERTED,PENDING,APPROVED,REJECTED,TRANSFERRED,ROLLBACK,DONE,OVERTIME_CLOSE,OVERTIME_RECOVER"`
	OperateTime    string          `json:"operate_time,omitempty"      desc:"Status change time in milliseconds" kind:"timestamp_ms"`
}
