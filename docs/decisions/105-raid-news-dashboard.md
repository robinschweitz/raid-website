# ADR 105 — Raid-News auf der Übersicht

Status: Accepted (Owner 2026-10-28)

## Entscheidung

Raidassistenz und höhere Rollen dürfen eigene Raid-News mit Titel, Text, Wichtigkeit und Veröffentlichungsstatus verwalten. Veröffentlichte Raid-News sind auf der Startseite nachlesbar. Wichtige Nachrichten erscheinen bei angemeldeten Spielern beim Öffnen der Übersicht als Dialog und werden nach einmaliger Bestätigung für genau diesen Spieler nicht erneut gezeigt. Neue Meldungen können erneut angezeigt werden.

## Begründung

Organisatorische Mitteilungen wie Rotation bei überfüllten Raids sollen zuverlässig gesehen werden, ohne die bestehenden automatischen Umfrage-/Raid-Benachrichtigungen zu ersetzen.

## Umsetzung und Grenzen

Bestätigungen werden serverseitig pro Spieler und Nachrichten-ID gespeichert. Entwürfe bleiben für berechtigte Mitarbeiter sichtbar. Nachrichten werden ausschließlich als Text gerendert. Eine Änderung einer schon bestätigten Nachricht ist keine neue Nachricht; wenn sie erneut erscheinen soll, muss eine neue Meldung veröffentlicht werden.

## Folge

Neue Migration und API sowie ein Dashboard-Bereich sind nötig. Tests für Rollen-/Besitzergrenzen, Speichern und Anzeige sind vor Freigabe erforderlich.
