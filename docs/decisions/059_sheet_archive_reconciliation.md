# ADR 059: Adopt Sheet archive outcomes atomically

Status: Accepted — owner instruction, 2026-10-14.

Amends ADR 057's website-only archival restriction and ADR 058's legacy cutover
block. The owner explicitly changed the rule: when the Sheet has archived a raid,
the website must also consider it archived, with its loot and already booked EP.
This applies to later Sheet archives as well as ICC25-35.

## Decision

- Presence in **Raid Archive**, with a valid consistent source timestamp, proves
  Sheet archival. Loot Archive alone does not. PLANNED/OPEN mapped website raids
  transition to COMPLETED with both completion/archive timestamps together.
- New Sheet history can be imported as archived history. Existing recurring raids
  may be bound through their proven imported series and exact raid name. Arbitrary
  website name collisions, cancellations and inconsistent archived states conflict.
- Loot, EP adjustments, archive state, expired unused locks, provenance, source
  baselines, ledger baselines, receipts and audit all share the existing outer
  reconciliation transaction. Preflight executes this same path and rolls it back.
- The Sheet's explicit participant EP balances are the post-archive target.
  Append only the difference from the website ledger; do not compute another
  +10 completion reward or debit enhanced loot again. Preserve the old ledger.
  Zero openings are explicitly recorded where a target is reconciled, preventing
  a later extra opening import. Reports expose before/target/difference values.
- A missing participant EP target conflicts when a no-main-spec award is due.
  If source MAIN/ENHANCED loot proves no reward is due in every newly archived
  participation, preserve that player's existing website balance and report the
  missing source value. Never infer zero from a blank cell.
- EP changes outside the newly archived participants remain source conflicts.
  A website ledger changed since the last matching source/website balance conflicts
  with a different Sheet target. An unchanged-source sync does not acknowledge a
  differing website balance. An already equal target needs no further adjustment.
- New no-ID loot is accepted only during the first proven archive adoption, using
  existing content/fingerprint/occurrence identities. Existing mapped awards must
  still match their source. Later edits/removals/multiplicity changes conflict;
  rows already recorded on the website without source identity are not duplicated.
- Old imports without source baselines can bootstrap only during a proven pending
  archive: old loot mappings/fingerprints, raid mappings/rosters and live character
  values are checked against the DB. Unrelated EP differences conflict. Missing
  provenance or historical report gaps remain blockers.
- Original-import provenance may be referenced by the authorized copy through an
  append-only scope relation, after the original setup's exact raid is found in
  the copy. The original mappings remain immutable; no original Sheet is fetched,
  written or configured as a continuous sync target. Read-only preflight rolls
  back this relation too.
- A Sheet archive is an **adoption of existing history**. It does not start a new
  recurrence chain or automatically create a successor; an already planned next
  raid remains intact. A subsequent website archive of the completed record never
  rewards again or schedules a new successor. Normal website completion retains
  its existing rewards/lock/successor behavior.
- The old setup block is skipped during adoption of its raid, and never copied
  into a successor. New setup naming/ownership is not inferred from an unlabeled
  spreadsheet block.
- API is Admin-only; the configured sync actor/reason remain mandatory. Automatic
  scheduling remains a separate opt-in. All remote reads/writes remain copy-only;
  exports remain limited to the seven Website mirror tabs.

## Verification

Self-contained isolated-DB tests cover positive/negative/zero targets, missing
zero openings, old/original provenance, later raids in the same series, multiple
retries, website re-archive, concurrent website EP (including intervening sync),
roster/loot/source conflicts, late rollback, lock expiry and export-failure retry.
An external evidence harness additionally tests the authorized-copy ICC25-35
snapshot with 25 participants and 28 no-ID loot rows, website pre-archive EP and
Sheet post-archive EP. No real archival, deployment, merge or production mutation
is performed during validation.
