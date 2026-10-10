# ADR 042: Rising-Gods-linked character creation

Status: Accepted

## Decision

Interactive character creation must use a Rising-Gods profiler character. Admin profile character
creation and raid quick-add no longer accept manually entered character names, classes, or specs for
new characters. The server loads the selected Rising-Gods profile and stores the local character from
that profile data.

Character claiming continues to use the Rising-Gods-backed registration-code flow from ADR 040.
Rising-Gods profile provenance is stored on the local character so the same external profile cannot be
silently attached to another player.

Legacy Google Sheet imports remain allowed to create historical characters without Rising-Gods
profiles. During merge, the importer attempts an exact-name Rising-Gods lookup and stores provenance
when a usable matching profile is found. Failed or ambiguous lookups do not block historical import and
do not overwrite existing character data.

## Consequences

The website no longer creates new live characters from free-text identity data. Rising-Gods lookup is
the source for name, class, and specs in interactive creation flows.

Historical imports preserve their source data first. Rising-Gods linking during import is best-effort
metadata enrichment only.
