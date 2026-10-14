# Sheet-Sync sicher vorbereiten

ICC25-35 ist im Sheet bereits mit Loot und Abschluss-EP archiviert; die Website
hat noch den aktiven Raid ohne Loot und mit EP vor dem Abschluss. **Nicht nochmals
auf der Website abschließen.** ADR 059 übernimmt diesen Sheet-Abschluss direkt:
Archivstatus, Loot und EP-Ausgleich werden gemeinsam verarbeitet. Weitere
Archivierungen im Sheet folgen derselben Regel.

## Prüfen und aktivieren

1. Eine konsistente SQLite-Sicherung über die Backupfunktion erstellen. Eine
   laufende WAL-Datenbank nicht nur durch Kopieren der `.db`-Datei sichern.
2. Neue Migrationen 058/059 zuerst auf einer separaten Datenbankkopie anwenden.
   Sie ergänzen Baseline-/Provenienztabellen; sie archivieren und buchen nichts.
3. Testserver ausdrücklich mit dieser Testdatenbank und ausschließlich der
   freigegebenen Sheet-Kopie konfigurieren:

   ```sh
   SHEET_SYNC_ENABLED=true
   SHEET_SYNC_AUTORUN=false
   SHEET_SYNC_SHEET_ID=10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8
   SHEET_SYNC_INTERVAL=10m
   SHEET_SYNC_REASON="Geprüfter Abgleich mit der freigegebenen Kopie"
   # Servicekonto sicher serverseitig bereitstellen, nur die Kopie freigeben.
   # DATABASE_URL ausdrücklich auf die separate Testdatenbank setzen.
   ```

4. Als Admin unter Importe „Vorabprüfung ohne Änderungen“ verwenden oder
   `POST /api/v1/sheet-sync/preflight` aufrufen. Die Vorschau zeigt geplante
   Archivübernahmen und je Spieler Website-Saldo, Sheet-Zielsaldo und Differenz.
   Domänen, Ledger, Baselines, Zuordnungen und Importreceipts bleiben unverändert.
5. Bei `ready=false` Blocker beheben; keine Identitäten oder Baselines per SQL
   erfinden. Widersprüchlicher Website-Loot, Aufstellungen und parallele EP-Änderungen
   sind bewusste Stopps, keine Aufforderung zum Überschreiben.
6. Bei `ready=true` einen manuellen Abgleich in der Testumgebung und identische
   Wiederholung prüfen. Erst nach erfolgreicher Prüfung des tatsächlichen
   Datenbankstands und gesicherter Ausgangsdaten den geplanten Rollout durchführen
   und den automatischen Zeitplan bewusst mit `SHEET_SYNC_AUTORUN=true` aktivieren.

## Übernahmeregeln

- Raid Archive belegt den Abschluss; Loot Archive allein genügt nicht.
- Ein bekannter importierter Raid oder ein exakt benannter Raid der bekannten
  importierten Serie wird wiederverwendet. Keine zweite Version von ICC25-35.
- Fehlende Loot-IDs dürfen beim ersten Archivübergang über dauerhafte
  Inhalt-/Vorkommensidentitäten übernommen werden. Spätere Änderungen daran
  bleiben Konflikte. Bekannter Website-Loot ohne Zuordnung wird nicht dupliziert.
- Sheet-EP sind für Teilnehmer des übernommenen Abschlusses der Zielstand.
  Nur Differenzen werden als neue Ledger-Buchungen ergänzt. Die normale
  Website-Abschlussprämie und Enhanced-Kosten werden dabei nicht erneut verbucht.
- Fehlende EP-Zielwerte sind ein Blocker, wenn eine Abschlussprämie nötig ist.
  Belegter Main-/Enhanced-Loot ohne Zielsaldo erhält vorhandene EP unverändert;
  dies wird gemeldet. Beispielsweise fehlt im geprüften Snapshot Skopins Saldo.
- Alte Imports ohne Baseline können nach vollständiger Prüfung des belegten
  Altbestands eine Baseline erhalten. Ursprüngliche Import-Zuordnungen dürfen in
  der Datenbank für die Kopie referenziert werden; das Original-Sheet bleibt tabu.
- Übernommene Sheet-Historie erzeugt keinen neuen Folge-Raid. Ein bereits geplanter
  Nachfolger bleibt erhalten; alte Setup-Daten werden nicht in ihn übertragen.
- Ein erneuter Website-Abschluss des übernommenen Raids bleibt ohne neue Prämien
  oder neue Folge-Raids. Normale Website-Abschlüsse anderer aktiver Raids behalten
  ihr bestehendes Verhalten.

Jeder Lauf prüft Quelle, Altbestand und Website-Mirrors erneut. Bei Fehler bleibt
`last_success` der letzte erfolgreiche Lauf; `last_error` beschreibt den Konflikt.
Ein Exportfehler nach dem DB-Commit wird ohne erneute Archiv-/EP-Effekte wiederholt.
Das ist kein vollständiger editierbarer Zwei-Wege-Abgleich: historische Löschungen,
Umbesetzungen, Charakteränderungen und manuell geänderte Website-Mirrors werden
weiterhin als Konflikte behandelt.
