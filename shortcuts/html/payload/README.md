# HTML publication payload

This package owns the dependency scanners, bounded collection, credential-file checks, entry mapping and content fingerprint used by `html +publish`.

It was copied from the implementation at `83ebce82998b33cd5e9622369805499e192b7ade` so HTML can evolve independently. It must not import `shortcuts/apps/deploy`; HTML-specific changes belong here. Apps directory traversal, app naming, build and deployment orchestration are not included.

HTML uses a 20,000,000-byte publication budget. The command-level 99-file limit and recovery guidance remain in the parent HTML package.
