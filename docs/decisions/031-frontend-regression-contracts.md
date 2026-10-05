# ADR 031: Frontend regression contracts after the dashboard refresh

## Decision

The owner requested repair of the pre-existing frontend test failures.

Regression tests follow the current dashboard widget layout and shared raid banner,
while continuing to verify selection of the actual next raid, stored composition,
real poll votes, and explicit empty/error states. They do not require the superseded
dashboard section structure or prohibit the existing empty notifications panel.

Catalog tests resolve both metadata and item requests. They verify source attribution,
read-only access across roles, failure of either request, retry, and rejection of stale
responses. Results appear only after both reads complete.

German text inside JavaScript string expressions uses Unicode characters rather than
HTML entities, which React otherwise displays literally. This corrects dashboard
empty/error/loading text and login/logout pending labels without changing workflows.
