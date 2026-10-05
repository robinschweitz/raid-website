# ADR 035: Charakterempfänger im Loot Archive

Status: Accepted

## Kontext

Die Spalte `Spieler` im Sheet-Tab `Loot Archive` enthält Charakternamen.
Der Owner hat dies ausdrücklich bestätigt, unter anderem für `Opinoid`.
Die bisherige Behandlung als Spielername erzeugte falsche Spielerzuordnungen
und ließ den historischen Charakterbezug am Loot leer.

## Entscheidung

- Der Import löst den Empfänger über den Charakter und dessen Besitzer aus
  `Charaktere` beziehungsweise dem bereits geplanten `Raid Archive` auf.
  EP- und Token-Zuordnungen folgen dem Besitzer, nicht dem Charakternamen.
- Wie bei bisher unbekannten Raid-Archive-Charakteren wird ein nur im Loot
  genannter Charakter mit gleichnamigem Spieler angelegt und im Report gemeldet.
  Klasse, Spec und Anwesenheit werden nicht aus dem Loot erfunden.
- Loot speichert sowohl die Charakter-ID als auch den historischen Namen aus
  der Quellzeile (`awarded_to_character_name`). Die Anzeige bevorzugt diesen
  Namen und nutzt sonst den vorhandenen Raid-Roster-Snapshot.
- Umbenennungen und sachfremde Loot-Korrekturen erhalten den gespeicherten Namen.
  Eine explizite Empfängerkorrektur entfernt den alten Namen; die Anzeige nutzt
  dann den Snapshot des neu ausgewählten Raid-Charakters.
- Quell-Fingerprints und Import-IDs bleiben unverändert. Bestehende Imports
  werden weiterhin dedupliziert und nicht automatisch überschrieben.
  Die Migration ergänzt nur das leere Namensfeld; eine Bestandskorrektur ist
  ein gesonderter, expliziter und auditierter Vorgang gemäß ADR 008/032.

## Prüfung

Regressionen prüfen abweichende Charakter-/Spielernamen, Tokenzuordnung,
Loot ohne Roster, historische Namen bei Umbenennung und Notizkorrektur,
Empfängerwechsel sowie die Anzeige des Sheet-Namens im Frontend.
