# IM raw capabilities and escape hatches

Use these capabilities only when the matching shortcut does not cover the requested operation. Read the leaf `--help` before execution; the examples below are the source for IM affordance navigation, not a replacement for generated flags, risk, identity, or completion contracts.

## Chat members

```bash
lark-cli im chat.members delete --chat-id <chat_id> --data '{"id_list":["<open_id>"]}'
lark-cli im chat.members get --chat-id <chat_id>
lark-cli im chat.members bots --chat-id <chat_id> --as bot
```

## Message operations

```bash
lark-cli im messages forward --message-id <message_id> --receive-id-type chat_id --data '{"receive_id":"<chat_id>"}' --as bot
lark-cli im messages delete --message-id <message_id>
lark-cli im messages merge_forward --receive-id-type chat_id --data '{"receive_id":"<chat_id>","message_id_list":["<message_id1>","<message_id2>"]}' --as bot
lark-cli im messages read_users --message-id <message_id> --user-id-type open_id
lark-cli im messages urgent_app --message-id <message_id> --user-id-type open_id --data '{"user_id_list":["<open_id>"]}' --as bot
lark-cli im messages urgent_phone --message-id <message_id> --user-id-type open_id --data '{"user_id_list":["<open_id>"]}' --as bot
lark-cli im messages urgent_sms --message-id <message_id> --user-id-type open_id --data '{"user_id_list":["<open_id>"]}' --as bot
```

## Card update and top notice

```bash
lark-cli api POST /open-apis/interactive/v1/card/update --as bot \
  --data '{"token":"<token>","card":{"type":"template","data":{"template_id":"<template_id>"}}}'
lark-cli api POST /open-apis/im/v1/chats/<chat_id>/top_notice/put_top_notice --as bot \
  --data '{"chat_top_notice":{"type":"message","message_id":"<message_id>"}}'
```

## Pins, threads, and chats

```bash
lark-cli im pins create --data '{"message_id":"<message_id>"}'
lark-cli im pins delete --message-id <message_id>
lark-cli im pins list --chat-id <chat_id>
lark-cli im threads forward --thread-id <thread_id> --receive-id-type chat_id --data '{"receive_id":"<chat_id>"}' --as bot
lark-cli im chats get --chat-id <chat_id>
lark-cli im chats update --chat-id <chat_id> --data '{"join_message_visibility":"only_owner"}'
lark-cli im chats create --data '{"name":"project chat"}'
lark-cli im chats link --chat-id <chat_id> --data '{"validity_period":"week"}'
```
