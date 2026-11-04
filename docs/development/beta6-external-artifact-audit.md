# Beta.6 Phase 3c-c — protected restore artifact reference audit

**Status:** CI-only, no operational restore activation. After merged PR #108.

The private `auditRestoredArtifactReferencesAt` helper reads a previously validated SQLite candidate journal and verifies that every **referenced** staged override metadata manifest and receipt publication has matching protected filesystem evidence. It uses existing root-only nofollow project directory and receipt-outbox readers, validates file ownership/mode/single-link properties, verifies content hashes and canonical sanitized receipt format, and fails closed on missing or tampered artifacts.

An interrupted stage intent without its manifest, or an incomplete receipt publication, is not silently interpreted as completed desired state. A matching assessment returns `rows_match_unqualified` with `BlocksNewWork=true`.

**This is only a one-way reference audit.** It does **not** enumerate every independent override or receipt file to detect **newer, unreferenced** artifacts that are absent from an old backup. That inverse-inventory check, a host-wide maintenance/quiescence proof, real Docker/Compose reconciliation, and an operator-approved offline restore activation remain separate hard gates. Do not use a matching reference audit as a restore authorization.

Tests execute only on disposable root-owned CI fixtures and cover valid references, missing/tampered/symlinked manifests and receipts, pending-only receipts and the persistent block on new work. No production host paths, Compose files or Docker operations are changed.

B6-03 tracking issue #83 stays open, and B6-08 off-host restore acceptance remains deferred. No merge, release or deployment without explicit operator approval.
