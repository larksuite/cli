# VC Participant Audio User Authentication Design

## Goal

Allow `vc +meeting-participant-mute` and `vc +meeting-participant-unmute` to run with either User or Bot identity.

## Contract

- Declare `AuthTypes` as `user` and `bot` for both shortcuts.
- Use `vc:meeting.bot.manage:write` for both identities.
- Keep the existing `POST /open-apis/v1/bots/mute` and `POST /open-apis/v1/bots/unmute` routes.
- Keep the existing `meeting_id`, `target_user_id`, and `user_id_type` request contract.
- Keep mute success as a completed result and unmute success as a request-sent result.
- Preserve existing typed API error handling.

## Implementation

Change only the shared participant-audio shortcut identity declaration. Do not add identity-specific request branches because both identities use the same scope, endpoint, parameters, body, and output.

## Tests

1. Update the shortcut contract test first to require `AuthTypes: [user, bot]` and confirm RED.
2. Exercise both mute and unmute with `--as user`, asserting the same HTTP method, path, query, body, and output contract used by Bot identity.
3. Keep the existing Bot execution assertions as regression coverage.
4. Run the focused VC package tests and participant-audio dry-run tests after the minimal implementation.

## Delivery

Base the change on the latest Codebase `feat/agent_employee_vc` branch. Commit the behavior change separately and update that same branch after focused verification.
