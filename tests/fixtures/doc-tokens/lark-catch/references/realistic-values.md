# Fixture: values check-doc-tokens.sh MUST report

Every line below is shaped like a real Lark identifier while being transparently
constructed. The check keys on shape, so the fixture needs the shape and nothing
else; copying an actual value out of the reference docs would move the problem
here rather than remove it.

The fixture must also not be a gitleaks hit itself, or the job this check feeds
would go red on the check's own test data. Two constraints follow from
.gitleaks.toml, and both are invisible to check-doc-tokens, which has no entropy
rule and no length rule beyond its own minimums:

- Values are built from a repeated four-character run, keeping Shannon entropy
  far below the `generic-api-key` threshold. Line 3 sits behind a `token` key
  and would otherwise be a textbook match for that rule.
- The application id is 18 characters after `cli_`, not the 16 a real one
  carries, so it falls outside this repo's `lark-bot-app-id` regex.

If a change to the script stops reporting any of these, it has lost coverage it
used to have.

1. Legacy prefixed token: `wikcnAbc1Abc1Abc1`
2. Open id in a response: "member_id": "ou_00112233445566778899aabbccddeeff"
3. Prefix-less Base token: "base_token": "Qw3rQw3rQw3rQw3rQw3rQw3r"
4. Partially masked token: --base-token bascn***************Qw3rT
5. Prefix-less token on a flag: --app-token Nq7bNq7bNq7bNq7bNq7bNq7b
6. Application id: "app_id": "cli_00112233445566aabb"
7. Stub glued onto a real value: "page_token": "Zx9pZx9pZx9pZx9pZx9pZx9pxxxx"
