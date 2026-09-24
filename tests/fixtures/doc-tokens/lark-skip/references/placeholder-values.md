# Fixture: values check-doc-tokens.sh must NOT report

These are the placeholder conventions the reference docs already use. Reporting
any of them would make the check noise, and noise is how a red check gets
ignored -- each line here was a false positive at some point in the script's
history.

1. Conventional stubs: `tblxxxxxxxx` `fldXXXXXXXX` `recXXXXXXXXXXXX` `boxcnxxxx` `blkcnXXXX`
2. Angle brackets: --base-token <base_token>
3. Named example: wikcn_EXAMPLE_TOKEN doccn_EXAMPLE_TOKEN
4. Your-own stub: your_token_here
5. Short and obviously fake: "user_id": "ou_12345"
6. Prose mentioning the flag: pass the base_token you copied from the Base URL to --base-token.
7. Head-only redaction stub: --base-token MAGObxxxxx
8. English words starting with a prefix: recommended, recipient, tblue
9. Readable stub with no digits: --to-user-id ou_new_speaker_open_id
10. All-digit identifier: --meeting-id 7628568141510692381
11. Lowercase example key: "page_token":"example_page_token"
12. RFC 4122 sample UUID: "card_id": "550e8400-e29b-41d4-a716-446655440000"
13. Digits either side of a redacted middle: --meeting-id 69xxxxxxxxxxxxx28
