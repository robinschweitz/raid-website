# ADR 112: Several raid leaders per series

Status: Accepted. Amends ADR 109 (the series keeps one leader as the default).

## Context

ADR 109 lets a raid list several raid leaders, but the series kept exactly one. Every raid
of a series that two people lead together needed its co-leader added by hand.

## Decision (owner, 2026-11-03)

- A raid series has one or more raid leaders, in order. The first stays in
  `raid_series.raid_leader_id`; the others are stored in `raid_series_leaders`, owned by the
  `raidseries` package. Each leader is an existing player, listed once.
- The series API takes and returns the whole list as `leader_ids`. A request that sends only
  `raid_leader_id` changes the first leader and keeps the others.
- Every raid created from the series copies the list: the create form, planning ahead, and the
  successor at archive time. A raid created by hand with another first leader keeps that leader
  first and adds the series' other leaders after it.
- Changing a series does not change raids that already exist; their list is edited per raid
  (FR-10.17).
- Leaders still grant no permissions.
