# ADR 069: Raid workspace polish

Status: Accepted. Amends ADR 068.

## Decision

Picker inputs for attendance characters and catalog items use the full form width, and the
clear action sits directly next to the field. Suggestions show names only: the item list is
already limited to one boss, difficulty and raid size, and a character ID or item ID is added
only when two entries would otherwise look identical. Typing an exact item ID still selects
that item. Saved-record identities in the correction editor keep showing the item ID. Success messages stay
visible while the user scrolls, can be closed, and are cleared when the user switches the raid
tab; they do not use timers. While a roster filter is active, empty roles and the bench say
"Keine Treffer" instead of "Noch nicht besetzt", and "Filter zurücksetzen" sits with the match
count. On small screens the raid tabs form a two-column grid instead of a scrolling strip, and
the attendance role selector has a visible label.

Only loot boss headings stay sticky. Roster column headings are no longer sticky: the columns are
short and have different heights, so their headings floated over partly hidden members.

## Consequences

The raid page remains dependency-free and the visual language is unchanged. Messages and filters
are local UI state only; API payloads, authorization and raid history are unaffected.

## Nachtrag: Boss und Schwierigkeit bleiben nach der Vergabe

Nach einer neuen Loot-Vergabe bleiben Boss, Schwierigkeit und Dropliste gewählt; Item, Empfänger,
Methode, Notizen und Grund werden zurückgesetzt. So lassen sich mehrere Items desselben Bosses
hintereinander erfassen. Nach einer Korrektur wird weiterhin alles zurückgesetzt.

## Nachtrag: Formulare oben, Empfänger-Suche, Hinweise

- Hinzufügen-Formulare (Loot erfassen, Teilnahme hinzufügen, Neuer Raid, Neuer Spieler) stehen als
  eingeklappter Abschnitt oberhalb der jeweiligen Liste, damit sie bei langen Listen ohne Scrollen erreichbar sind.
- Der Loot-Empfänger ist wie Item und Charakter ein Suchfeld mit Vorschlägen; eine genau eingegebene
  Charakter-ID wählt ebenfalls aus. Nur bei gleichen Namen erscheint `#ID`.
- Neben „Vergeben“ erklärt ein Hinweis, was fehlt. Items, die in diesem Raid schon vergeben wurden,
  tragen „bereits n× vergeben“; mehrfaches Vergeben bleibt erlaubt (mehrere Exemplare, BoE).
- Enter im Item-Feld fokussiert den Empfänger; Erfolgsmeldungen schließen sich nach 8 Sekunden.
