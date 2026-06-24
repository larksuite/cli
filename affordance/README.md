# Affordance

Per-command usage guidance for the CLI, authored as one markdown file per domain
(`<service>.md`). It is surfaced in `lark-cli <command> --help` and in the
`schema` output, and read directly at runtime (lazy, cached) — there is no build
step. Maintain these files alongside `skills/` and `shortcuts/`.

## Format

A small, fixed markdown subset; each file describes one domain:

    # <domain>            optional `> skill: <name>` applies to every command below
    ## <command>          the command as typed, minus `lark-cli <domain>`
    <lead paragraph>      when to use this command
    ### Avoid when        when not to use it / which command to use instead
    ### Prerequisites     what you must have first (e.g. an id, and where it comes from)
    ### Tips              gotchas and constraints
    ### Examples          **description** lines, each followed by a fenced command
    ### <other heading>   a custom section; flows through verbatim

Reference another command with `[[command]]` — it renders as `command` in help.
Under `Avoid when` it means "use that one instead"; under `Prerequisites`
("… from [[command]]") it means "get the input there first".

## Example

    ## messages get
    Fetch the full content of a single message by id.

    ### Avoid when
    - Reading several at once → use [[messages batch_get]]

    ### Prerequisites
    - message_id from [[messages list]]

    ### Examples

    **Fetch one message**
    ```bash
    lark-cli mail user_mailbox.messages get --message-id "<id>"
    ```

## Notes

- Write plain prose; the only convention is wrapping command references in `[[ ]]`.
- Keep it concise and high-signal — don't restate field/flag names, id types, or
  anything the schema and flags already show; the agent infers the rest.
- Command-form headings resolve to method ids via the registry, so plural resource
  names (`messages`) map to the singular method id (`message`) automatically.
