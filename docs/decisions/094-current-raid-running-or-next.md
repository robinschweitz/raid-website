# ADR 094: Current raid means running or next upcoming

Status: Accepted.

## Context

The owner clarified on 2026-10-23 that the "Aktueller Raid" menu item must always lead to the next raid that is due or currently taking place. ADR 088 kept the most recent completed raid selected until another began, which led to stale navigation.

## Decision

This ADR supersedes only the "Aktueller Raid" selection rule of ADR 088; the boss-order decision remains unchanged.

- Select only non-archived raids in PLANNED or OPEN status. COMPLETED and CANCELLED raids are not eligible.
- If any eligible raid has already started (real start preferred, otherwise scheduled start), select the most recently started one. A past scheduled start counts as running until the raid is completed, cancelled or archived.
- Otherwise select the eligible raid with the earliest future start. For equal times choose the lowest raid ID; for already started raids at equal times choose the highest ID.
- Ignore raids without both start timestamps. Return 404 `no_current_raid` when no raid qualifies.
- Both desktop and mobile menu links keep using `/raids/current` and the existing authenticated API and replace-style redirect.
