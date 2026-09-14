# VC E2E Coverage

## Agent Calendar Lifecycle

| Status | Cmd | Coverage | Notes |
| --- | --- | --- | --- |
| Dry-run | `vc +meeting-join --action start` | `vc_agent_meeting_actions_dryrun_test.go::TestVCAgentMeetingActionsDryRun/start and join` | Verifies the bot join endpoint and `action=2`. |
| Dry-run | `vc +meeting-invite` | `vc_agent_meeting_actions_dryrun_test.go::TestVCAgentMeetingActionsDryRun/invite all suggested` | Verifies the Agent invite endpoint and long `meeting_id`. |
| Dry-run | `vc +meeting-end` | `vc_agent_meeting_actions_dryrun_test.go::TestVCAgentMeetingActionsDryRun/end meeting` | Verifies the bot end endpoint and that `--dry-run` bypasses the real-execution confirmation gate. |
| Live | `vc +meeting-end` | `vc_meeting_end_test.go::TestVCMeetingEndLiveE2E` | Fixture-gated host success plus cohost and ordinary-user denial cases. |

## Meeting Recording Controls

| Status | Cmd | Coverage | Notes |
| --- | --- | --- | --- |
| Dry-run | `vc +meeting-recording-start` | `vc_meeting_recording_control_dryrun_test.go::TestVCRecordingControlDryRun/+meeting-recording-start` | Verifies the PATCH endpoint and omitted request body. |
| Dry-run | `vc +meeting-recording-stop` | `vc_meeting_recording_control_dryrun_test.go::TestVCRecordingControlDryRun/+meeting-recording-stop` | Verifies the PATCH endpoint and omitted request body. |
| Blocked Live | `vc +meeting-recording-start` |  | Starting a recording creates a persistent Minutes artifact that the current fixture cannot delete. |
| Blocked Live | `vc +meeting-recording-stop` |  | A successful stop requires a recording started by the same unsafe workflow. |

## Participant Audio Controls

| Status | Cmd | Coverage | Notes |
| --- | --- | --- | --- |
| Dry-run | `vc +meeting-participant-mute` | `vc_meeting_participant_audio_dryrun_test.go::TestVCMeetingParticipantAudioDryRun/+meeting-participant-mute` | Verifies the shared User/Bot `/open-apis/vc/v1/bots/mute` endpoint, query, and body contract without sending `device_id`. |
| Dry-run | `vc +meeting-participant-unmute` | `vc_meeting_participant_audio_dryrun_test.go::TestVCMeetingParticipantAudioDryRun/+meeting-participant-unmute` | Verifies the shared User/Bot `/open-apis/vc/v1/bots/unmute` request endpoint, query, and body contract without claiming a completed unmute. |
| Live request | `vc +meeting-participant-mute --as user` | `vc_meeting_participant_audio_workflow_test.go::TestVCMeetingParticipantAudioLiveE2E/mute` | Verifies that the live backend accepts the host's mute request; no independent microphone-state readback is available. |
| Live request | `vc +meeting-participant-unmute --as user` | `vc_meeting_participant_audio_workflow_test.go::TestVCMeetingParticipantAudioLiveE2E/request_unmute` | Verifies that the live backend accepts the unmute request without claiming that the participant accepted it. |

## Participant Removal

| Status | Cmd | Coverage | Notes |
| --- | --- | --- | --- |
| Live | `vc +meeting-participant-kickout` | `vc_meeting_participant_kickout_test.go::TestVCMeetingParticipantKickoutLiveE2E` | Fixture-gated host/cohost success, ordinary-user denial, and partial-result cases. |

## Remaining Live E2E Blockers

The Agent Calendar `meeting-join`, `meeting-invite`, and bot-identity
`meeting-end` paths still need an isolated fixture that can create an
Agent-enabled meeting with the application bot as Host and a distinct eligible
invitee.

The bot-identity participant-audio path also remains uncovered because the
current fixture contract does not provide a tenant token bound to an active bot
participant and a separate target participant.

Recording-control Live E2E remains blocked because starting a recording creates
a persistent Minutes artifact. Ending the disposable meeting does not delete
that artifact, and the current fixture has neither artifact readback nor exact
cleanup.

The user-identity participant-audio Live E2E proves request acceptance, not the
participant's final microphone state, because no independent state readback is
available.

## Required Fixture

1. A dedicated test app with the Agent Calendar feature enabled for its meeting
   owner.
2. A disposable app-calendar event that creates a VC meeting and makes that app
   bot Host.
3. A second user open_id scoped to the same test app for invite and audio-control
   assertions.
4. A tenant token bound to the bot, independent participant-state readback, and
   cleanup that restores participant state and ends the meeting.
5. A disposable recording fixture with Minutes artifact readback and exact
   artifact deletion.

## Substitute Evidence

On 2026-08-24, the gray-lane manual lifecycle created a bot-owned Calendar
event, started it, sent an `ALL_SUGGESTED` invite, ended it as the Host bot,
deleted the event, and verified that the active-meeting list was empty.

The bot-identity participant-audio path is covered by
`shortcuts/vc/vc_meeting_participant_audio_test.go` against the HTTP stub and by
the binary dry-run test above. These checks lock identity, method, path, query,
body, and error handling without claiming live-backend coverage.

Recording controls are covered by
`shortcuts/vc/vc_meeting_recording_control_test.go` against the HTTP stub and by
the binary dry-run tests above. These checks do not create persistent tenant
state and do not claim live-backend coverage.
