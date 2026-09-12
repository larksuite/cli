# Record export contract

The NDJSON exporter validates matrix dimensions and preserves column alignment.
Known Base v3 field types use their declared cell shape: array fields normalize
null to `[]`, checkboxes normalize null to `false`, and timestamps normalize to
RFC3339. Buttons have no record value and export as null with physical type `null`.

Unknown field types export with manifest `field_type: "not_support"` and
`physical_type: "json"`. Their decoded JSON values pass through without cell
normalization, including null, booleans, numbers, arrays, and nested objects.
The same passthrough applies when the API explicitly returns `not_support`.
This preserves JSON values, not original whitespace or object key order.
Fallback columns report null counts without assuming a string or array shape.

The original API field types remain in the source schema so a type change
between pages still fails the export, even if both types map to `not_support`.
Malformed matrices and invalid cells of known types remain errors.
