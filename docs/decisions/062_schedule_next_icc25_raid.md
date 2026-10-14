# ADR 062 — Automatische ICC25-Nachfolgeplanung aus aktuellem Raidsetup

Status: Accepted — Klarstellung des Eigentümers, 2026-10-14.

## Anlass

ADR 059 importierte Sheet-Archivierungen, ohne einen Folge-Raid zu planen.
ADR 061 aktualisierte nur einen bereits vorhandenen geplanten Raid und konnte
deshalb `ICC25-36` nicht erzeugen, wenn `ICC25-35` bereits im Google Sheet
archiviert worden war. Der Eigentümer stellt ausdrücklich klar:
`ICC25 Raidsetup` enthält **die Teilnehmer des nächsten geplanten Raids**;
nach `ICC25-35` soll die Website `ICC25-36` anzeigen.

## Entscheidung

1. `Raid Archive` bleibt für die **belegte historische Archivierung** zuständig:
   historisches Loot und EP werden wie bisher atomar importiert, ohne erneut
   Abschlussbelohnungen zu vergeben.
2. Im selben rollbackfähigen Reconcile-Lauf verarbeitet `SyncCurrentSetup`
   den ersten nummerierten A:F-Block von `ICC25 Raidsetup` als **aktuelle
   bevorstehende Aufstellung**. Gibt es genau einen aktiven, nicht archivierten
   Raid der explizit importierten ICC25-Serie, werden nur fehlende bekannte
   Teilnehmer darin ergänzt (keine Löschung/Überschreibung).
3. Gibt es keinen aktiven Raid, darf der Sync ihn **anlegen**, wenn genau eine
   zuvor explizit importierte Serie `ICC25` mit der überprüften Vorlage
   `ICC25-{number}`, dem 25er-ICC-Katalog und einem gültigen Wiederholungsplan
   existiert. Der letzte nummerierte Raid im freigegebenen `Raid Archive`
   muss durch eine tatsächliche Importzuordnung auf einen archivierten
   Website-Raid verweisen. Seine höchste Nummer bestimmt die **nächste**
   Nummer (`ICC25-35` → `ICC25-36`), die Serienregeln bestimmen den
   nächsten zukünftigen Termin mit DST-sicherer Wiederholung.
4. Es wird genau ein `PLANNED`-Raid in derselben Serie mit
   `previous_raid_id`, dem geplanten Termin und dem festgelegten Namen
   erstellt. Namen, Termine und Verlinkung werden gegen Dubletten und
   widersprüchliche Serienzähler validiert, dann wird der Zähler gespeichert.
   Audit-Eintrag, Raid-Erstellung, Setup-Zeilen, Archive und EP gehören
   zur selben Reconcile-Transaktion; Preflight rollt alles zurück.
5. Die bisherige Bedingung »Sheet-Aufstellung muss sich vom archivierten
   Raid unterscheiden« entfällt: Identische Teilnehmer können legitimerweise
   auch im nächsten Raid spielen. Das Live-Sheet ist die Aufstellung dieses
   geplanten Raids, keine historische Snapshot-Diff-Datei.
6. Keine Erzeugung bei fehlendem oder mehrdeutigem Setup-Mapping, einem
   unbekannten Serienplan, abweichenden Katalogdaten oder einem
   Namens-/Terminkonflikt. Unbekannte Charaktere und widersprüchliche
   vorhandene Teilnahmen stoppen die gesamte Änderung. Der Original-Sheet
   bleibt streng unverändert und ausgeschlossen.

Die neuen `created_raid`, `raid_name` und `scheduled_start`-Felder der
Setup-Vorschau zeigen die Planung bereits vor dem Übernehmen; der nächste
geplante Raid erscheint nach erfolgreichem Lauf regulär in der Website.
Diese Entscheidung ersetzt ausschließlich die »kein Nachfolger«-Regel aus
ADR 059 und die Änderungsnachweis-Bedingung aus ADR 061.
