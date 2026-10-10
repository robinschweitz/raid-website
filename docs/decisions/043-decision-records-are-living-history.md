# ADR 043: Decision records are living history

Status: Accepted

## Decision

Accepted ADRs describe the current project decision history. They are authoritative while they remain
current, but they are not permanent constraints against later owner decisions.

When the owner changes, rejects, or narrows an existing decision, the repository must record that
change in the same work:

- Add a new ADR with the next number.
- State which older ADR or rule it supersedes, amends, or narrows.
- Update `AGENTS.md` if it summarizes the changed rule.
- Do not silently rewrite old ADRs to hide that the decision changed.

If an old ADR appears to force unwanted behavior, stop and ask the owner instead of implementing that
behavior only because the old ADR exists. Once the owner answers, record the new decision explicitly.

## Consequences

Agents and contributors should treat ADRs as active documentation that must evolve with the product.
Outdated decisions are corrected by adding a clear superseding record, not by ignoring the ADR set and
not by blindly enforcing known-bad decisions.
