# ADR 044: Rising-Gods lookup scope and performance

Status: Accepted

## Decision

Interactive Rising-Gods character lookup is scoped to Alliance level-80 characters. This applies to
registration, character claiming, Admin-linked character creation, and raid quick-add character
creation. The application filters Rising-Gods profile searches with Alliance and level-80 criteria and
rejects loaded profiles that do not satisfy the same rule.

Users may paste a normal Rising-Gods profile URL such as
`https://db.rising-gods.de/?profile=eu.rising-gods.charactername`. The server resolves that URL to
the numeric profiler id and then loads the profile through the faster profiler load endpoint. Broad
name search remains available for ordinary character-name input.

Rising-Gods profile listviews are capped and a broad Alliance level-80 list is not complete enough to
act as a local character index. The application must not treat the first broad listview page as the full
set of available characters. Ordinary name searches therefore use targeted Rising-Gods filters with the
user's query, plus caching and request coalescing for repeated searches. Successful profile
URL resolutions and profile loads may also be cached for short operational windows. Caching is a
performance optimization only; local character creation still stores the loaded profile provenance and
never fabricates manual live-character data.

When a cached targeted search is known complete because it returned fewer rows than the Rising-Gods
listview cap, a more specific later query may be answered by filtering that cached result locally. A
capped result must not be reused as complete for a more specific query.

Measurements on 2026-10-11 showed that the Rising-Gods profile listview search takes about 3-5 seconds
on the Rising-Gods side for every query, independent of filter selectivity, result size or concurrency,
while profile loads take about 0.5 seconds. Name search results are therefore cached for 12 hours and,
once expired, served stale for up to 7 days while a background refresh runs. Search results are only
used for picking a character; the chosen profile is always loaded again before anything is stored, so a
stale search row never ends up in local data. The number of cached searches is bounded because the
search endpoint is public. Shared upstream fetches run detached from the request that started them, so
one cancelled request does not fail the other callers waiting for the same lookup. A profile load that
lacks talent data takes the spec from a cached search row before falling back to a live search.
Proactive cache warming was removed by ADR 048; upstream searches are made only for user-requested
queries.

Legacy import Rising-Gods enrichment uses exact per-character lookup. Failed or ambiguous matches
remain best-effort metadata enrichment and do not block historical imports.

## Consequences

Horde and non-level-80 Rising-Gods profiles are treated as unusable for interactive live-character
flows. Historical imports remain governed by ADR 042 and may preserve source characters without
Rising-Gods provenance.

Repeated or concurrent lookups for the same profile or search query should avoid duplicate upstream
Rising-Gods requests where practical.
