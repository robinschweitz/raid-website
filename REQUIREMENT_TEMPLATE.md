# Requirement Template — raid-website implementation issues

Use one copy for each leaf implementation issue (one coherent behaviour, control, endpoint,
workflow, or data flow; at most about 2 engineer-days). Replace every bracketed placeholder.
Write `N/A` only after checking that a field does not apply, and `TO BE DECIDED` when the answer
is open (then the issue is blocked by a `REQ-DECISION-*` issue). Normative words follow
RFC 2119 / RFC 8174: MUST, MUST NOT, SHOULD, SHOULD NOT, MAY. Split compound behaviour.

## Metadata

- **ID**: [REQ-<CAPABILITY>-### , e.g. REQ-AUTH-010]
- **Title**: [Short imperative name]
- **Status**: [Draft | Ready | In Progress | Implemented | Verified]
- **Priority**: [Critical | High | Medium | Low]
- **Type**: [Functional | Security | Privacy | Performance | Reliability | Operational | Cleanup]
- **Source**: [FR-* / NFR-* / SC-* / DR-* / DESIGN rule / ADR / owner decision that this issue implements]
- **Epic / Workstream**: [#issue]

## Requirement

- **Statement**: [One testable sentence: what the system MUST or MUST NOT do after this issue.]
- **Rationale**: [Why — reference the owning spec ID instead of restating it.]
- **Current behaviour**: [What the code does today, with file paths; `None` for new features.]
- **Out of scope**: [What this issue deliberately does not do, or `None`.]

## Traceability

- **Requirements**: [FR-* / NFR-* IDs in `REQUIREMENTS.md`, or `N/A`]
- **Design**: [`DESIGN.md` section or component, or `N/A`]
- **Architecture**: [`ARCHITECTURE.md` component, DR-* rule, trust boundary TB-*]
- **Security**: [SC-* rules in `SECURITY.md`, or `N/A`]
- **ADR**: [New ADR required? Name the ADR it supersedes or amends, or `None`]

## Scope

- **Surfaces**: [Web client | API | Domain service | Repository / migration | Deployment | Multiple]
- **Packages / files**: [e.g. `internal/auth`, `web/src/pages/Login.tsx`]
- **Endpoints / pages**: [Method and path, or route, or `N/A`]
- **Actors and roles**: [Visitor | GUEST | MEMBER | RAIDASSISTENT | RAIDLEADER | ADMIN | system]
- **Data touched**: [Tables and fields; mark personal data and secrets]
- **Migration**: [New numbered migration pair? Existing data handling?]

## Security Context

- **Trust boundary**: [TB-* where untrusted input enters, or `N/A`]
- **Untrusted inputs**: [Body fields, headers, pasted addon text, Rising-Gods data …]
- **Enforcement point**: [Server component that enforces the rule (never only the client)]
- **Abuse case**: [What a lower role, a visitor, or a forged request might try, or `N/A`]
- **ASVS 5.0.0 chapter**: [e.g. V6 Authentication; chapter level unless a requirement ID is verified]

## Acceptance Criteria

Each criterion is one observable pass/fail outcome in Given/When/Then form. Cover success,
invalid input, unauthorised access, and failure behaviour where they apply.

1. **AC-01 Success**: Given [state], when [actor does X], then [observable result].
2. **AC-02 Invalid input**: Given [state], when [bad input], then [4xx with code `…`] and nothing changes.
3. **AC-03 Unauthorised**: Given [role one step too low / no session], when [action], then [403 / 401] and nothing changes.
4. **AC-04 Failure**: Given [dependency or persistence failure], when [action], then [rollback, safe error, no partial state].
5. **AC-05 …**: [Add only when needed.]

## Failure Behaviour

- **Invalid input**: [Status and stable error code]
- **Authorisation failure**: [Status; whether existence may be revealed]
- **External dependency failure**: [e.g. 502 `rising_gods_unavailable`, or `N/A`]
- **System error**: [Transaction rollback; generic `internal` error (SC-023)]
- **Audit**: [Entity type and action written (FR-26), or why none]

## Test Strategy

- **Go unit / service tests**: [Business rules and error paths]
- **HTTP tests** (`internal/httpapi`): [Permission boundary, status codes, response shape]
- **Frontend tests** (`web/tests`): [Rendering and states, or `N/A`]
- **End-to-end** (`web/e2e`): [Browser journey and screenshots, or `N/A`]
- **AC mapping**: [Test name for each AC-*]

## Done When

- [ ] All acceptance criteria pass and are covered by tests.
- [ ] Gates in `AGENTS.md` pass.
- [ ] Owning spec files updated (remove "not built" notes this issue resolves).
- [ ] ADR added if a decision changed; `web/src/release.ts` version and changelog for user-facing changes.
- [ ] Guide text and screenshots updated if a described screen changed.

## Dependencies and Notes

- **Blocked by**: [#issues, including `REQ-DECISION-*`, or `None`]
- **Blocks**: [#issues or `None`]
- **Prohibited approaches**: [e.g. client-only checks, logging secrets, new dependency without approval]
- **Open decisions**: [Must be `None` before Status is Ready]
