# ADR 013: Character Names Mutable With History

## Decision
Character names are mutable (player can rename ingame). Track `former_names`/name history. No DB-level global UNIQUE(name); use stable IDs.

## Consequences
Handles renames while preserving audit.
