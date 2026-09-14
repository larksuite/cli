# Minutes CLI E2E Coverage

## Share Permission

| Status | Cmd | Coverage | Notes |
| --- | --- | --- | --- |
| Dry-run | `minutes +share-permission` | `minutes_share_permission_test.go::TestMinutesSharePermission_DryRun` | Verifies the POST endpoint, minute token path parameter, and omitted request body. |

## Live E2E Blocker

`minutes +share-permission` grants access to every participant associated with a
Minutes resource. The current E2E environment does not provide a disposable
Minutes fixture with a known participant set or a cleanup operation that can
revoke exactly the permissions granted by this command. Running it against a
shared Minutes resource would leave tenant-visible permission changes behind.

## Required Fixture

1. A disposable Minutes resource owned by the test identity.
2. A deterministic participant set whose permissions are absent before the test.
3. Independent permission readback and cleanup that revokes only permissions
   granted by the test, or deletes the disposable Minutes resource.

## Substitute Evidence

`shortcuts/minutes/minutes_share_permission_test.go` exercises the successful
request and output path against the HTTP stub. The binary dry-run test above
locks the method, path, and empty-body contract. These checks do not claim
live-backend coverage.
