# IM CLI E2E Coverage

## Metrics
- Denominator: 30 leaf commands
- Covered: 12
- Coverage: 40.0%

## Summary
- TestIM_ChatUpdateWorkflow: proves `im +chat-create`, `im +chat-update`, and `im chats get`; key `t.Run(...)` proof points are `update chat name as bot`, `update chat description as bot`, and `get updated chat as bot`.
- TestIM_ChatsGetWorkflow: proves `im chats get` on a fresh chat fixture via `get chat info as bot`.
- TestIM_ChatsLinkWorkflow: proves `im chats link` via `get chat share link as bot`.
- TestIM_ChatMessageWorkflowAsUser: proves the user chat message flow through `create chat as user`, `send message as user`, and `list chat messages as user` with the created message ID and content asserted from read-after-write output.
- TestIM_MessageGetWorkflowAsUser: proves user message readback through `batch get message as user` after creating a fresh chat and sending a unique message.
- TestIM_MessageReplyWorkflowAsBot: proves threaded reply flow through `reply to message in thread as bot` and `list thread replies as bot`, reading back the reply from `im +threads-messages-list`.
- TestIM_MessagesResourcesDownloadDryRun: pins the `im +messages-resources-download` request shape (GET, `/open-apis/im/v1/messages/:message_id/resources/:file_key`, `type=file|image`) and the output-path rejection for absolute and parent-escaping paths, without calling the API.
- TestIM_MessageResourceDownloadWorkflowAsBot: proves `im +messages-resources-download` end to end through `download file resource through the bounded first request` — a 320 KiB fixture is uploaded with `im +messages-send --file`, its `file_key` is read back off the message, and the downloaded bytes are compared byte for byte; the command always sends an initial Range request, but the output does not reveal whether the endpoint answered 206 or 200, so this proves the round trip rather than which path ran. Multi-part continuation is pinned by unit tests. Skips when the test app lacks the IM resource upload scope.
- TestIM_MessagesSendAudioDryRunRejectsNonOpus: proves the `im +messages-send --audio` dry-run validation rejects non-Opus local audio before upload, with typed validation metadata and recovery guidance.
- TestIM_MessageForwardWorkflowAsUser: proves UAT-backed API forwarding through `im messages forward` and `im threads forward` using a fresh message/thread fixture; skips the forward assertions when the current test app/UAT lacks IM forward permission.
- Coverage prerequisite (structured):
  - blocked_case: im.search.stable_fixture_required
  - affected_commands: `im +chat-search`, `im +messages-search`
  - coverage_rule: deterministic search assertions must use stable pre-existing fixtures
  - next_fixture_requirement: stable historical chat/message fixtures

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | im +chat-create | shortcut | im/chat_message_workflow_test.go::TestIM_ChatMessageWorkflowAsUser/create chat as user; im/chat_workflow_test.go::TestIM_ChatUpdateWorkflow; im/chat_workflow_test.go::TestIM_ChatsGetWorkflow; im/chat_workflow_test.go::TestIM_ChatsLinkWorkflow; im/message_get_workflow_test.go::TestIM_MessageGetWorkflowAsUser; im/message_reply_workflow_test.go::TestIM_MessageReplyWorkflowAsBot | `--name`; `--type private` | covered via workflow setup with created chat IDs asserted |
| ✓ | im +chat-messages-list | shortcut | im/chat_message_workflow_test.go::TestIM_ChatMessageWorkflowAsUser/list chat messages as user; im/message_reply_workflow_test.go::TestIM_MessageReplyWorkflowAsBot/list thread replies as bot; im/tips_examples_dryrun_test.go::TestIMTipsFirstExampleDryRunChatMessagesList | `--chat-id`; `--start`; `--end` | reads back created message and discovers thread ID |
| ✕ | im +chat-search | shortcut |  | none | deterministic coverage requires a stable pre-existing chat fixture |
| ✓ | im +chat-update | shortcut | im/chat_workflow_test.go::TestIM_ChatUpdateWorkflow/update chat name as bot; im/chat_workflow_test.go::TestIM_ChatUpdateWorkflow/update chat description as bot | `--chat-id`; `--name`; `--description` | |
| ✓ | im +messages-mget | shortcut | im/message_get_workflow_test.go::TestIM_MessageGetWorkflowAsUser/batch get message as user | `--message-ids` | verifies sent message content by ID |
| ✓ | im +messages-reply | shortcut | im/message_reply_workflow_test.go::TestIM_MessageReplyWorkflowAsBot/reply to message in thread as bot | `--message-id`; `--text`; `--reply-in-thread` | reply is read back via thread list |
| ✓ | im +messages-resources-download | shortcut | im/messages_resources_download_dryrun_test.go::TestIM_MessagesResourcesDownloadDryRun; im/message_resource_download_workflow_test.go::TestIM_MessageResourceDownloadWorkflowAsBot/download file resource through the bounded first request; im/tips_examples_dryrun_test.go::TestIMTipsFirstExampleDryRunResourcesDownload | `--message-id`; `--file-key`; `--type`; `--output` | uploads a 320 KiB fixture via `+messages-send --file`, reads file_key back off the message, downloads it and compares bytes directly; the command sends an initial Range request, though the output cannot show whether the endpoint answered 206 or 200; multi-part continuation is covered by unit tests |
| ✕ | im +messages-search | shortcut |  | none | freshly sent messages were not indexed deterministically in UAT time for a stable read-after-write proof |
| ✓ | im +messages-send | shortcut | im/chat_message_workflow_test.go::TestIM_ChatMessageWorkflowAsUser/send message as user; im/message_get_workflow_test.go::TestIM_MessageGetWorkflowAsUser; im/message_reply_workflow_test.go::TestIM_MessageReplyWorkflowAsBot; im/message_audio_dryrun_test.go::TestIM_MessagesSendAudioDryRunRejectsNonOpus; im/tips_examples_dryrun_test.go::TestIMTipsFirstExampleDryRunMessagesSend | `--chat-id`; `--text`; `--audio ./voice.mp3 --dry-run` | live text sends feed follow-up reads; dry-run pins non-Opus audio validation before upload |
| ✓ | im +threads-messages-list | shortcut | im/message_reply_workflow_test.go::TestIM_MessageReplyWorkflowAsBot/list thread replies as bot | `--thread` | proves threaded reply is persisted |
| ✕ | im chat.members create | api |  | none | no member mutation workflow yet |
| ✕ | im chat.members get | api |  | none | no member get workflow yet |
| ✕ | im chats create | api |  | none | only covered indirectly through `+chat-create` |
| ✓ | im chats get | api | im/chat_workflow_test.go::TestIM_ChatUpdateWorkflow/get updated chat as bot; im/chat_workflow_test.go::TestIM_ChatsGetWorkflow/get chat info as bot | `chat_id` in `--params` | |
| ✓ | im chats link | api | im/chat_workflow_test.go::TestIM_ChatsLinkWorkflow/get chat share link as bot | `chat_id` in `--params`; `validity_period` in `--data` | |
| ✕ | im chats list | api |  | none | command absent from current command surface; kept for historical tracking |
| ✕ | im chats update | api |  | none | only covered indirectly through `+chat-update` |
| ✕ | im images create | api |  | none | no image upload workflow yet |
| ✕ | im messages delete | api |  | none | no recall workflow yet |
| ✓ | im messages forward | api | im/message_forward_workflow_test.go::TestIM_MessageForwardWorkflowAsUser/forward message with api command as user | `message_id`; `receive_id_type`; `uuid`; `receive_id` | forwards a fresh message back into the test chat using UAT |
| ✕ | im messages merge_forward | api |  | none | no merge-forward workflow yet |
| ✕ | im messages read_users | api |  | none | no read-user workflow yet |
| ✓ | im threads forward | api | im/message_forward_workflow_test.go::TestIM_MessageForwardWorkflowAsUser/forward thread with api command as user | `thread_id`; `receive_id_type`; `uuid`; `receive_id` | forwards a fresh thread back into the test chat using UAT |
| ✕ | im pins create | api |  | none | pin workflows not covered |
| ✕ | im pins delete | api |  | none | pin workflows not covered |
| ✕ | im pins list | api |  | none | pin workflows not covered |
| ✕ | im reactions batch_query | api |  | none | reaction workflows not covered |
| ✕ | im reactions create | api |  | none | reaction workflows not covered |
| ✕ | im reactions delete | api |  | none | reaction workflows not covered |
| ✕ | im reactions list | api |  | none | reaction workflows not covered |
