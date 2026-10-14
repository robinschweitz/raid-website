# Sheet-Sync sicher vorbereiten

## Derzeitiger Produktionsstand

ICC25-35 ist im Sheet mit Loot archiviert, auf der Website jedoch noch aktiv,
ohne Loot. Diesen Raid nicht ohne vorherigen Loot-/EP-Abgleich archivieren.
Die Nachstellung ergab +250 EP ohne Loot gegenüber +160 EP mit den 28 Lootzeilen.
Auch die +160 EP können bereits im tatsächlichen Altbestand enthalten sein.

Der neue Code schützt den Bestand durch Konfliktstopps. Er repariert diese
historische Differenz nicht automatisch und ist kein Ersatz für die Prüfung
der tatsächlichen Datenbank.

## Vorbereitung auf einer konsistenten Datenbankkopie

1. Eine konsistente SQLite-Sicherung über die SQLite-Backupfunktion erstellen.
   Eine laufende WAL-Datenbank nicht durch Kopieren nur der `.db`-Datei sichern.
2. Die neue Migration 058 zuerst an dieser Kopie testen. Sie ergänzt nur
   Quellen-/Setupbaseline-Tabellen; sie bucht keine Punkte und verändert keine Raids.
3. Einen Testserver mit dieser Datenbankkopie betreiben. Servicekonto nur an der
   freigegebenen Sheet-Kopie berechtigen, Geheimnis ausschließlich serverseitig:

   ```sh
   SHEET_SYNC_ENABLED=true
   SHEET_SYNC_AUTORUN=false
   SHEET_SYNC_SHEET_ID=10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8
   SHEET_SYNC_INTERVAL=10m
   SHEET_SYNC_REASON="Geprüfter Abgleich mit der freigegebenen Kopie"
   # SHEET_SYNC_SERVICE_ACCOUNT_JSON sicher serverseitig bereitstellen
   # DATABASE_URL explizit auf die separate Testdatenbank setzen
   ```

4. Als Admin unter Importe „Vorabprüfung ohne Änderungen“ aufrufen. Alternativ
   `POST /api/v1/sheet-sync/preflight` mit einer Adminsession. Der API-Modus startet
   ohne `SHEET_SYNC_AUTORUN=true` keine Hintergrundläufe.
5. Bei `ready=false` keine Laufzeitaktivierung. Die Blocker sind der konkrete
   Arbeitsauftrag für den einmaligen Altbestandsabgleich, kein Fehler zum Umgehen.

## Für ICC25-35 muss vor Freigabe geklärt werden

- Welche `legacy_raid_mappings`/`legacy_setup_mappings` verweisen auf den tatsächlichen
  Website-Raid? Stammt die Provenienz vom Original oder schon von der Kopie?
- Sind alle 25 tatsächlichen Teilnehmer korrekt zugeordnet und die 28 Sheet-Lootzeilen
  noch vollständig fehlend? Wie werden die fehlenden Awards einmalig eindeutig und
  mit dauerhafter Quellidentität übernommen, ohne bereits erfassten Loot zu duplizieren?
- Welche EP-Buchungen/Eröffnungsstände sind vorhanden und welche enthalten bereits
  ICC25-35? Fehlende Punkte dürfen einmalig ergänzt werden; vorhandene Belohnungen
  dürfen nicht erneut vergeben werden.
- Wie wird der Abschluss ausschließlich über eine explizite Website-Aktion durchgeführt,
  ohne historische EP zu replayen oder einen Folge-Raid zu duplizieren?
- Welcher geprüfte Quellenstand wird anschließend zur Baseline? Ein nachträglich
  geänderter aktueller Sheetstand darf nicht blind als alte Importbasis gelten.

Keine Produktionszuordnungen direkt per SQL umschreiben, keine alten Lootzeilen
pauschal mit neuen IDs versehen und keine Baseline-Tabelle manuell füllen, um einen
Konflikt zu übergehen. Die einmalige Übernahme braucht einen vorher prüfbaren Plan,
Actor/Reason, Ledger-Kompensationen soweit erforderlich und Retry-Nachweise.

## Freigabe des laufenden Abgleichs

Nach dem bestätigten Altbestandsabgleich: Preflight auf der vorbereiteten Kopie
mit `ready=true`, einmaliger manueller Testlauf und identische Wiederholung ohne
doppelte Domain-Einträge, Snapshot-Readback, gesicherter Produktionsstand und
erneute Preflight-Prüfung dort. Erst dann bewusst `SHEET_SYNC_AUTORUN=true` setzen.

Jeder tatsächliche Lauf prüft Quellenbaseline, Historienarchivierung, Setup und
Mirrorkonflikte erneut. Bei Fehler: `last_error` lesen und Ursache klären.
`last_success` bleibt der letzte tatsächlich erfolgreiche Lauf.

Die Quelle ist keine vollständige editierbare zweite Datenbank: Änderungen an
historischen Punkten, Specs, Eigentümern oder entfernten Zeilen sind Konflikte;
sie werden nicht als Last-Writer-Wins übernommen. Website-Spiegeltabs sind
websiteautoritativ und dürfen nicht manuell als zweite Eingabemaske gepflegt werden.
