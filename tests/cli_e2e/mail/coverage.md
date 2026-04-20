# Mail CLI E2E Coverage

## Metrics
- Denominator: 62 leaf commands
- Covered: 13
- Coverage: 21.0%

## Summary
- TestMail_DraftLifecycleWorkflowAsUser: proves a self-contained user draft workflow across `mail user_mailboxes profile`, `mail +draft-create`, `mail user_mailbox.drafts list`, `mail user_mailbox.drafts get`, `mail +draft-edit`, and `mail user_mailbox.drafts delete`; key `t.Run(...)` proof points are `get mailbox profile as user`, `create draft with shortcut as user`, `list draft as user`, `get created draft as user`, `inspect created draft as user`, `update draft subject with shortcut as user`, `inspect updated draft as user`, `delete draft as user`, and `verify draft removed from list as user`.
- TestMail_SendWorkflowAsUser: proves a self-contained self-mail workflow across `mail +send`, `mail +triage`, `mail +message`, `mail +messages`, `mail +thread`, `mail +reply`, and `mail +forward`; key `t.Run(...)` proof points are `send mail to self with shortcut as user`, `find self sent mail in triage as user`, `get sent message as user`, `get received message as user`, `get both self sent messages as user`, `get self send thread as user`, `reply to received message with shortcut as user`, `inspect reply draft as user`, `forward received message with shortcut as user`, and `inspect forward draft as user`.
- Blocked area: `mail +reply-all` is still uncovered because the self-send workflow produces only self-recipient traffic and reply-all’s recipient expansion becomes degenerate after self-address exclusion; `+signature`, `+watch`, event commands, and many raw message/thread mutation APIs still need dedicated tenant-aware workflows.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | mail +draft-create | shortcut | mail_draft_lifecycle_workflow_test.go::TestMail_DraftLifecycleWorkflowAsUser/create draft with shortcut as user | `--subject`; `--body`; `--plain-text` | creates a new self-owned draft without relying on external recipients |
| ✓ | mail +draft-edit | shortcut | mail_draft_lifecycle_workflow_test.go::TestMail_DraftLifecycleWorkflowAsUser/inspect created draft as user; mail_draft_lifecycle_workflow_test.go::TestMail_DraftLifecycleWorkflowAsUser/update draft subject with shortcut as user; mail_draft_lifecycle_workflow_test.go::TestMail_DraftLifecycleWorkflowAsUser/inspect updated draft as user | `--draft-id`; `--mailbox me`; `--inspect`; `--set-subject` | shortcut proves readback projection and subject update |
| ✓ | mail +forward | shortcut | mail_send_workflow_test.go::TestMail_SendWorkflowAsUser/forward received message with shortcut as user; mail_send_workflow_test.go::TestMail_SendWorkflowAsUser/inspect forward draft as user | `--message-id`; `--to`; `--body`; `--plain-text` | uses self-generated inbox message as source and inspects forwarded draft projection |
| ✓ | mail +message | shortcut | mail_send_workflow_test.go::TestMail_SendWorkflowAsUser/get sent message as user; mail_send_workflow_test.go::TestMail_SendWorkflowAsUser/get received message as user | `--mailbox me`; `--message-id` | verifies both SENT and INBOX copies after self-send |
| ✓ | mail +messages | shortcut | mail_send_workflow_test.go::TestMail_SendWorkflowAsUser/get both self sent messages as user | `--mailbox me`; `--message-ids` | batch reads both sent and received message copies |
| ✓ | mail +reply | shortcut | mail_send_workflow_test.go::TestMail_SendWorkflowAsUser/reply to received message with shortcut as user; mail_send_workflow_test.go::TestMail_SendWorkflowAsUser/inspect reply draft as user | `--message-id`; `--body`; `--plain-text` | creates reply draft from self-generated inbox message and inspects quoted content |
| ✕ | mail +reply-all | shortcut |  | none | self-send traffic leaves no stable non-self recipient set for deterministic reply-all assertions |
| ✓ | mail +send | shortcut | mail_send_workflow_test.go::TestMail_SendWorkflowAsUser/send mail to self with shortcut as user | `--to`; `--subject`; `--body`; `--plain-text`; `--confirm-send` | self-send creates both sent and inbox copies for follow-up assertions |
| ✕ | mail +signature | shortcut |  | none | signature availability is mailbox-configuration dependent |
| ✓ | mail +thread | shortcut | mail_send_workflow_test.go::TestMail_SendWorkflowAsUser/get self send thread as user | `--mailbox me`; `--thread-id` | verifies readback of the sent-message thread created by self-send |
| ✓ | mail +triage | shortcut | mail_send_workflow_test.go::TestMail_SendWorkflowAsUser/find self sent mail in triage as user | `--mailbox me`; `--query`; `--max`; `--format data` | polls until self-sent subject becomes searchable and captures sent/inbox message ids |
| ✕ | mail +watch | shortcut |  | none | requires websocket event subscription setup and external mail delivery |
| ✕ | mail multi_entity search | api |  | none | requires deterministic searchable contact entities |
| ✕ | mail user_mailbox.drafts create | api |  | none | only covered indirectly through `mail +draft-create` |
| ✓ | mail user_mailbox.drafts delete | api | mail_draft_lifecycle_workflow_test.go::TestMail_DraftLifecycleWorkflowAsUser/delete draft as user | `user_mailbox_id`; `draft_id` in `--params` | explicit lifecycle delete plus read-after-delete list check |
| ✓ | mail user_mailbox.drafts get | api | mail_draft_lifecycle_workflow_test.go::TestMail_DraftLifecycleWorkflowAsUser/get created draft as user | `user_mailbox_id`; `draft_id` in `--params` | asserts persisted draft id, subject, and draft state |
| ✓ | mail user_mailbox.drafts list | api | mail_draft_lifecycle_workflow_test.go::TestMail_DraftLifecycleWorkflowAsUser/list draft as user; mail_draft_lifecycle_workflow_test.go::TestMail_DraftLifecycleWorkflowAsUser/verify draft removed from list as user | `user_mailbox_id`; `page_size` in `--params` | proves create visibility and delete removal |
| ✕ | mail user_mailbox.drafts send | api |  | none | draft send needs recipient-side or send-status assertions to be deterministic |
| ✕ | mail user_mailbox.drafts update | api |  | none | only covered indirectly through `mail +draft-edit` |
| ✕ | mail user_mailbox.drafts cancel_scheduled_send | api |  | none | requires a scheduled-send draft lifecycle |
| ✕ | mail user_mailbox.event subscribe | api |  | none | requires event subscription setup |
| ✕ | mail user_mailbox.event subscription | api |  | none | requires event subscription setup |
| ✕ | mail user_mailbox.event unsubscribe | api |  | none | requires event subscription setup |
| ✕ | mail user_mailbox.folders create | api |  | none | folder lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.folders delete | api |  | none | folder lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.folders get | api |  | none | folder lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.folders list | api |  | none | folder lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.folders patch | api |  | none | folder lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.labels create | api |  | none | label lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.labels delete | api |  | none | label lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.labels get | api |  | none | label lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.labels list | api |  | none | label lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.labels patch | api |  | none | label lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.mail_contacts create | api |  | none | contact lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.mail_contacts delete | api |  | none | contact lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.mail_contacts list | api |  | none | contact lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.mail_contacts patch | api |  | none | contact lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.message.attachments download_url | api |  | none | requires an existing message attachment |
| ✕ | mail user_mailbox.messages batch_get | api |  | none | requires existing message ids |
| ✕ | mail user_mailbox.messages batch_modify | api |  | none | requires existing messages and mailbox folders/labels |
| ✕ | mail user_mailbox.messages batch_trash | api |  | none | requires existing messages |
| ✕ | mail user_mailbox.messages get | api |  | none | requires an existing message id |
| ✕ | mail user_mailbox.messages list | api |  | none | requires deterministic existing folder or label message inventory |
| ✕ | mail user_mailbox.messages modify | api |  | none | requires existing messages and mailbox folders/labels |
| ✕ | mail user_mailbox.messages send_status | api |  | none | requires a sent message id |
| ✕ | mail user_mailbox.messages trash | api |  | none | requires an existing message id |
| ✕ | mail user_mailbox.rules create | api |  | none | rule lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.rules delete | api |  | none | rule lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.rules list | api |  | none | rule lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.rules reorder | api |  | none | rule lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.rules update | api |  | none | rule lifecycle left for a dedicated workflow |
| ✕ | mail user_mailbox.sent_messages get_recall_detail | api |  | none | requires a recallable sent message |
| ✕ | mail user_mailbox.sent_messages recall | api |  | none | requires a delivered sent message within recall window |
| ✕ | mail user_mailbox.settings send_as | api |  | none | mailbox alias availability is tenant-configuration dependent |
| ✕ | mail user_mailbox.threads batch_modify | api |  | none | requires existing threads and mailbox folders/labels |
| ✕ | mail user_mailbox.threads batch_trash | api |  | none | requires existing thread ids |
| ✕ | mail user_mailbox.threads get | api |  | none | requires an existing thread id |
| ✕ | mail user_mailbox.threads list | api |  | none | requires deterministic existing folder or label thread inventory |
| ✕ | mail user_mailbox.threads modify | api |  | none | requires existing threads and mailbox folders/labels |
| ✕ | mail user_mailbox.threads trash | api |  | none | requires an existing thread id |
| ✕ | mail user_mailboxes accessible_mailboxes | api |  | none | mailbox visibility differs by tenant and shared-mailbox configuration |
| ✓ | mail user_mailboxes profile | api | mail_draft_lifecycle_workflow_test.go::TestMail_DraftLifecycleWorkflowAsUser/get mailbox profile as user | `user_mailbox_id=me` in `--params` | proves current mailbox identity before draft lifecycle |
| ✕ | mail user_mailboxes search | api |  | none | requires deterministic searchable mailbox content |
