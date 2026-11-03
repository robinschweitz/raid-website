# ADR 109: Own specs, weekly profile reload, signups with several specs, several raid leaders

Status: Accepted. Amends ADR 032 (only Admins edit live characters), ADR 033 (one spec per
signup), and ADR 021 (one raid leader per raid).

## Context

Guild members asked for four changes (2026-11-03): sign up with more than one spec, refresh
their Rising-Gods data themselves, fix their main and second spec when the profile guesses
wrong, and name more than one raid leader. A character also showed a gear score of about 9000:
the Rising-Gods profile lists each slot's transmog appearance as an item of its own, and the
gear score counted it. That is a bug, not a decision; only the worn item counts now.

## Decision (owner, 2026-11-03)

- **Several specs per signup.** A signup holds one or more valid specs of the character's
  class. The form offers the character's main and second spec as checkboxes. Staff choose one
  of the signed-up specs when they accept; the first one is the default. Signups from before
  this change keep their single spec.
- **Own profile reload, once a week.** The owner (MEMBER and higher) of a linked character may
  reload its Rising-Gods profile at most once per 7 days per character; earlier attempts get
  `409 profile_reload_too_soon`. Admins stay unlimited (FR-6.14). Every reload records its
  time.
- **Own specs.** The owner may set main spec and second spec of an own character to any valid
  spec of its class; the second spec is optional and differs from the main spec. Name and
  class stay Admin-only. Once the owner has chosen, a profile reload keeps the specs and only
  updates name and gear score.
- **Several raid leaders, display only.** A raid has one or more raid leaders. The first comes
  from the series or the create form as before; staff may set the list of planned, open, and
  completed raids, also archived ones, so old raids can be fixed; an archived raid needs a
  reason (FR-26.5), and cancelled raids stay unchanged (FR-13.11). Leaders grant no
  permissions; rights stay with the role. The series keeps one leader as the default.
