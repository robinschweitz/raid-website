# ADR 019: Safe Incremental Imports

## Decision
The owner approved incremental imports and an optional permanent `Import ID` column
in Loot Archive. IDs must be unique within the sheet and remain unchanged for the same
source event. Different IDs distinguish legitimate identical drops.

Persist append-only provenance scoped by source, sheet, and tab. Unchanged IDs are skipped;
edited or duplicated IDs produce conflicts without updating existing awards. For old rows
without IDs, content fingerprints plus occurrence numbers retain multiplicity and tolerate
reordering. After the initial import, unseen no-ID content, removals, and multiplicity
changes require review rather than automatic insertion. Counters exclude explicit-ID rows.

A conflict rejects the entire merge with an explicit report and no domain writes.
Raid identity is also persisted by source and sheet; a name collision without provenance
is a conflict, not permission to insert into another raid. New rows in existing imported
raids require an explicit correction reason and use the audited correction transaction,
including EP eligibility differences. Cancelled and archived raids reject new rows.
Initial creation of a new imported raid seeds history without backfilling raid EP awards.
Do not reconstruct provenance for historical imports that lack it. Identical snapshots
return the stored report with `skipped=true`; when a historical report is unavailable,
return an explicit warning rather than inventing creation counts.

New merges require an error-free server preview for the source, sheet, and raw hash,
plus the matching `raw_hash` in the merge request. Preview metadata is persisted without
domain writes. Merge fetches again and rechecks source consistency and database conflicts
inside the write transaction. Successful reports are persisted with the import.

Sheet dates are interpreted explicitly in Europe/Berlin and converted to UTC. Loot uses
the source date as its award timestamp. Missing/invalid dates are reported as warnings
and retain the existing import-time fallback. Tokens derive only from the three item
names specified in ADR 014, never from a priority label alone.

## Consequences
The merge API accepts `sheet_id`, `raw_hash`, and an optional `reason` that is required
for additions to existing imported raids. Missing preview evidence yields
`preview_required`; a changed source yields `source_changed`. Corrections to imported
events belong in the audited correction workflow, not in an overwriting importer.
