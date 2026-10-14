# ADR 059: Website-to-Google-Sheets loot delivery

Status: Accepted by owner, 2026-10-13.

## Decision

- Manual loot writes for an explicitly linked Google Sheets raid enqueue a durable outbound operation in the same database transaction.
- A website award has the permanent sheet Import ID `website-award-<award-id>`. Corrections update that row; withdrawal retains the local award and writes `WITHDRAWN` to its sheet row.
- Delivery is administrator-triggered through the flush endpoint. Each attempt records status, count, error, and append-only local delivery evidence.
- The delivery ensures an append-only `Website Sync Audit` worksheet through Google Sheets `batchUpdate`, then appends operation evidence. An absent tab that cannot be created is a failed outbox operation, not a silent local success.

## Consequences

Google Sheets cannot atomically combine the Loot Archive write and audit append. Retrying after an unknown network result can therefore repeat an audit row; each row carries the durable operation ID. The loot row is idempotent by permanent Import ID.
