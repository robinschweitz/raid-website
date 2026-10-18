# ADR 080: Short recruitment setup text

Status: Accepted. Extends the recruitment text template.

## Decision

`{{SETUP}}` in the Gesuchtext lists every missing position by its full name ("Druide
Wiederherstellung, Priester Disziplin, Irgendein MDD, …"), which is too long for the raid search
chat. The owner asked for a compact form like "2 Heal (Baum, Diszi), 2 MDD (beliebig), 2 RDD (Mage,
SP)" and for the format to be changeable.

The template text already decides the wording, so the format becomes changeable through placeholders
rather than through new settings:

- `{{SETUP_KURZ}}`: missing positions grouped by role, in the order Tank, Heal, MDD, RDD. Each role
  shows its count and the short names of the missing specs, duplicates as "2x Eule" and specific
  wishes before "beliebig". A role that only misses "Irgendein …" slots shows "beliebig" without a
  count. Roles with nothing missing are left out; with nothing missing at all the text is "keine
  offenen Plätze".
- `{{TANK_SPECS}}`, `{{HEAL_SPECS}}`, `{{MDD_SPECS}}`, `{{RDD_SPECS}}`: only the short names of one
  role ("-" when nothing is missing), for custom sentences.
- `{{SETUP}}` is unchanged, so saved templates keep working.

The short names are the common chat names (MT, OT, Fury, Rogue, Unholy, Ret, Hunter, Hexer, Eule,
Mage, Ele, SP, Holy Pala, Resto Schami, Baum, Diszi; "beliebig" for "Irgendein …"). They are fixed in
the frontend. Making them editable per guild was considered and left out until someone needs
different names.

## Consequences

Purely a frontend change; no API or storage change. Raid leads switch to the short form by editing
their template.
