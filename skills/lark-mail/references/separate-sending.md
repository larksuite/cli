# Send separately

Use the existing mail service to deliver separately; the CLI never loops over recipients.

- Create a draft: `lark-cli mail +draft-create --to a@example.com --to b@example.com --subject Hello --body Hello --send-separately=true --as user`.
- Enable on a saved draft: `lark-cli mail +draft-edit --draft-id DRAFT_ID --send-separately=true --as user`.
- Cancel: use the same edit command with `--send-separately=false`.
- Read the saved setting: `lark-cli mail +draft-edit --draft-id DRAFT_ID --inspect --as user`; the output includes `is_send_separately` when supplied by the service.
- Send the saved draft: `lark-cli mail +draft-send --draft-id DRAFT_ID --yes --as user`. This uses the saved setting. To change it, edit successfully before sending.
- For compose-and-send, `+send` accepts the same flag; retain its existing confirmation and preview workflow.

Omitting the flag preserves saved state on edits and uses ordinary sending for new drafts. Explicit false is sent to the service. Create/update responses do not need to echo the setting; read the draft to confirm it. Existing account, recipient, Cc/Bcc, scheduling, and send restrictions remain enforced by the service. A successful request is not proof that every recipient received the message. On an uncertain send result, check the mailbox before retrying.

Draft edits have no optimistic locking: avoid concurrent editors. Never interpret a missing detail field as false.
