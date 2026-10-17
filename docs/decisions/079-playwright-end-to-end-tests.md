# ADR 079: Playwright end-to-end tests

Status: Accepted.

## Decision

The owner asked for end-to-end tests and screenshots of the raid planning feature. The frontend
regression tests (`web/tests`) render components against a mocked API and never start the server or a
browser, so they cannot show that the pieces work together.

`@playwright/test` is added as a **dev dependency only**; nothing ships to the browser or the Go binary.
`npm run e2e` (in `web/`) builds the frontend, builds and starts the real Go server on port 8765 against
a fresh SQLite database in `web/.e2e/`, and drives Chromium through the UI. Tests live in `web/e2e/`.

- Test data is created through the API as the bootstrap admin. Characters normally come from the
  Rising-Gods armory, which tests cannot reach, so the e2e seed writes character rows straight into the
  test database. No test-only endpoints or backdoors are added to the server.
- Each test step saves a screenshot to `web/e2e-screenshots/`; CI uploads them with the HTML report.
- CI runs the suite in a separate `e2e` job.

## Consequences

Features can be verified in a real browser and shown with screenshots. The e2e job adds a Chromium
download and about a minute to CI.
