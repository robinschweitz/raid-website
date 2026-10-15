# ADR 068: Raidgebundener Gesuchtext

Status: Accepted

## Decision

Der Gesuchtext gehört zum Workspace des jeweiligen aktiven Raids und ist kein
eigenständiger Navigationsbereich. Vorlage und Soll-Besetzung bleiben pro Raid im lokalen
Browser-Speicher abgelegt. Die Lückenberechnung verwendet ausschließlich die gespeicherte
Aufstellung dieses Raids und aktualisiert sie regelmäßig.

## Consequences

Der Gesuchtext ist im Tab `Gesuchtext` eines geplanten oder offenen Raids erreichbar.
Abgeschlossene und abgesagte Raids bieten keine Rekrutierungskonfiguration.

## Nachtrag: gemeinsame Vorlage im Backend

- Neue Tabelle `raid_recruitment_templates` (Migration 068): eine Vorlage pro Raidserie, bei Raids ohne Serie pro
  Raid. Endpunkte `GET`/`PUT /api/v1/raids/{id}/recruitment-template`, Rolle RAIDASSISTENT oder höher
  (Lesen und Schreiben). Gespeichert werden Text (höchstens 2000 Zeichen) und eine 5×5-Soll-Besetzung; jeder
  Speichervorgang wird als `raid_recruitment_template`/`save` protokolliert. Die gültigen Platzwerte prüft nur
  das Frontend, der Server nur Form und Länge.
- Die Oberfläche speichert ausdrücklich mit „Vorlage speichern“ (kein Speichern bei jedem Tastendruck). Eine
  bisher nur im Browser (`localStorage`) gespeicherte Vorlage wird als Entwurf übernommen, solange der Server
  noch keine hat.
- Die 15-Sekunden-Aktualisierung ruft nur noch `GET …/attendance` ab, statt die gesamte Raidseite neu zu laden.
