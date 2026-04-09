# IM CLI E2E Coverage

## Metrics

- Denominator: 29 leaf commands
- Covered: 15
- Coverage: 51.7%

## Summary

- Chat workflows prove `im +chat-create`, `im +chat-update`, `im +chat-search`, `im chats {get,list,link}`, and `im chat.members get`; key proof points are returned `chat_id`, updated chat fields, chat search success, and `share_link` retrieval.
- Message workflows prove `im +messages-send`, `im +chat-messages-list`, `im +messages-mget`, `im +messages-reply`, and `im +threads-messages-list`; key proof points include non-empty `message_id`, non-empty mget results, and thread listing after a thread reply.
- Reaction and pin workflows prove read paths with `im pins list`, `im reactions list`, and `im reactions batch_query`.
- Blocked area: `im +messages-search` is skipped in bot-only environments because it requires user login.
- Gap pattern: several commands are exercised only as structure checks or dry-run probes (`messages forward`, `messages merge_forward`, `messages read_users`, `images create`) and are not counted as covered.
- Gap pattern: direct mutation APIs such as `chat.members create/delete`, `pins create/delete`, `reactions create/delete`, and `messages delete` currently do not prove returned fields or post-mutation state strongly enough to count as covered.

## Command Table

| Status | Cmd | Type | Testcase | Key Parameter Shapes | Notes / Uncovered Reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | im +chat-create | shortcut | im/helpers_test.go::createChat; im/helpers_test.go::createChatWithBotManager; chat_workflow_test.go::TestIM_ChatCreateWithOptionsWorkflow/create chat with set-bot-manager; chat_workflow_test.go::TestIM_ChatCreateWithOptionsWorkflow/create public chat with description | `--name --type`; optional `--description`; optional `--set-bot-manager` | Covered through helper and workflow assertions on returned `chat_id`. |
| ✓ | im +chat-messages-list | shortcut | message_workflow_test.go::TestIM_ChatMessagesListWorkflow/list messages in chat | `--chat-id`; optional `--sort`; optional `--page-size` | |
| ✓ | im +chat-search | shortcut | chat_workflow_test.go::TestIM_ChatSearchWorkflow/search chat by name; chat_workflow_test.go::TestIM_ChatSearchWorkflow/search chat with sort | `--query`; optional `--sort-by` | first search path may skip when the backend does not return bot-created chats |
| ✓ | im +chat-update | shortcut | chat_workflow_test.go::TestIM_ChatUpdateWorkflow/update chat name; chat_workflow_test.go::TestIM_ChatUpdateWorkflow/update chat description | `--chat-id --name`; `--chat-id --description` | |
| ✓ | im +messages-mget | shortcut | message_workflow_test.go::TestIM_MessagesMgetWorkflow/batch get messages by ID; message_workflow_test.go::TestIM_MessagesResourcesDownloadWorkflow/send image message and download resource | `--message-ids` | |
| ✓ | im +messages-reply | shortcut | message_workflow_test.go::TestIM_MessagesReplyWorkflow/reply to message with text; message_workflow_test.go::TestIM_MessagesReplyWorkflow/reply to message with markdown; message_workflow_test.go::TestIM_MessagesReplyInThreadWorkflow/reply in thread; thread_workflow_test.go::TestIM_ThreadsMessagesListWorkflow/setup thread with reply | `--message-id --text`; `--message-id --markdown`; optional `--reply-in-thread` | |
| ✕ | im +messages-resources-download | shortcut |  | none | only conditionally executed in `TestIM_MessagesResourcesDownloadWorkflow`; no stable file assertion yet |
| ✕ | im +messages-search | shortcut |  | none | skipped in bot-only environments; requires user login |
| ✓ | im +messages-send | shortcut | chat_workflow_test.go::TestIM_ChatCreateSendWorkflow/send text message to chat; chat_workflow_test.go::TestIM_ChatCreateSendWorkflow/send markdown message to chat; chat_workflow_test.go::TestIM_ChatCreateSendWorkflow/send image message to chat; im/helpers_test.go::sendMessage; im/helpers_test.go::sendMarkdown; im/helpers_test.go::sendImage | `--chat-id --text`; `--chat-id --markdown`; `--chat-id --image` | |
| ✓ | im +threads-messages-list | shortcut | thread_workflow_test.go::TestIM_ThreadsMessagesListWorkflow/list thread messages; thread_workflow_test.go::TestIM_ThreadsMessagesListWorkflow/list thread messages with asc sort; thread_workflow_test.go::TestIM_ThreadsMessagesListWorkflow/list thread messages with page size | `--thread`; optional `--sort`; optional `--page-size` | |
| ✕ | im chat.members create | api |  | none | only structure-checked with invalid ids; no asserted success or persisted state |
| ✓ | im chat.members get | api | chat_workflow_test.go::TestIM_ChatMembersWorkflow/get chat members | params with `chat_id` | |
| ✕ | im chats create | api |  | none | only covered indirectly through `im +chat-create` |
| ✓ | im chats get | api | chat_workflow_test.go::TestIM_ChatsGetWorkflow/get chat info | params with `chat_id` | |
| ✓ | im chats link | api | chat_workflow_test.go::TestIM_ChatsLinkWorkflow/get chat share link | params with `chat_id`; request body with `validity_period` | |
| ✓ | im chats list | api | chat_workflow_test.go::TestIM_ChatsListWorkflow/list chats | no required params | |
| ✕ | im chats update | api |  | none | only covered indirectly through `im +chat-update` |
| ✕ | im images create | api |  | none | only dry-run command-structure check |
| ✕ | im messages delete | api |  | none | delete call succeeds, but no post-delete state proof yet |
| ✕ | im messages forward | api |  | none | structure-only check with invalid receiver id |
| ✕ | im messages merge_forward | api |  | none | structure-only check with invalid ids |
| ✕ | im messages read_users | api |  | none | structure-only check with invalid message id |
| ✕ | im pins create | api |  | none | no asserted returned fields or post-create state specific to the pinned message |
| ✕ | im pins delete | api |  | none | no post-delete verification yet |
| ✓ | im pins list | api | pin_workflow_test.go::TestIM_PinsWorkflow/list pinned messages | params with `chat_id` | |
| ✓ | im reactions batch_query | api | reaction_workflow_test.go::TestIM_ReactionsWorkflow/batch query reactions | request body with message ids | |
| ✕ | im reactions create | api |  | none | no asserted returned fields or post-create state specific to the new reaction |
| ✕ | im reactions delete | api |  | none | no post-delete verification yet |
| ✓ | im reactions list | api | reaction_workflow_test.go::TestIM_ReactionsWorkflow/list reactions for a message | params with `message_id` | |
