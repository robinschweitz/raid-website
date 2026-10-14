# ADR 061 — Live ICC25-Raidsetup für den aktuell anstehenden Raid

Status: Accepted — owner request, 2026-10-14.

## Motivation

Das Tabellenblatt `ICC25 Raidsetup` (erster nummerierter Block in A:F)
beschreibt die jeweils aktuelle Aufstellung. Bisher wurde es nur in den
ursprünglichen, provenienzgebundenen Raid übernommen. Sobald dieser archiviert
war, blockierte ein fester Vergleich mit dessen ID jeden späteren Nachfolger.

## Entscheidung

- Die bestehende explizit importierte Raidserie ist weiterhin die einzige
  Zielserie. Gibt es **genau einen** aktiven, nicht archivierten `PLANNED`-
  oder `OPEN`-Raid dieser Serie, ist er der aktuelle Ziel-Raid. Es wird
  kein neuer Raid angelegt, keiner umbenannt, keine Serie neu erstellt.
- Wird ein Nachfolger erstmals an das Sheet gebunden, muss der ursprünglich
  importierte Raid abgeschlossen und archiviert sein. Der Nachfolger benötigt
  einen späteren, explizit geplanten Starttermin. Ein zuvor nachweislich
  synchronisierter, archivierter Raid derselben Serie mit gespeicherter
  Setup-Baseline ist der bevorzugte Vergleich. Bei älteren Imports ohne
  Setup-Baseline darf alternativ die letzte tatsächlich archivierte
  Website-Aufstellung derselben Serie verglichen werden. Der aktuelle erste
  nummerierte Sheet-Block muss sich nachweislich von der vorherigen
  Aufstellung unterscheiden; eine unveränderte oder nicht belegbare
  Aufstellung wird nicht übernommen. Diese Ausnahme ist ausdrücklich für
  die Migration alter Erstimporte gedacht und erzeugt keine künstliche
  Provenienz über vergangene Sheetstände.
- Sobald die neue Zuordnung erstmals geprüft und angewendet wurde, wird
  ihre Quelle als eigene Baseline je Zielraid gespeichert. Folgeläufe
  gleichen neue bekannte Teilnehmer additiv ab; entfernte, geänderte,
  doppelte oder mehrdeutige Einträge bleiben Konflikte, nie stille
  Löschungen oder Überschreibungen. Die alten Baselines werden behalten.
- In der Admin-Vorabprüfung ist der geplante Zielraid mit Namen, ID und
  Anzahl Ergänzungen sichtbar. Die Vorabprüfung bleibt transaktional und
  schreibt nichts; alle bisherigen Sheet-Archivierungs-, Audit-, EP- und
  Provenienzregeln bleiben unverändert. Unbekannte Charaktere, parallele
  aktive Raids sowie fehlende Zuweisungsbeweise bewirken keine Änderung.

Die einmalige Setup-Provenienz `legacy_setup_mappings` bleibt unverändert
und zeigt weiterhin auf den ursprünglichen Raid. Das originale Google Sheet
bleibt vollständig ausgeschlossen; die neue Zuordnung gilt nur für die
freigegebene Arbeitskopie.
