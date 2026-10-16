export const appVersion = '1.0.12'

export const changelog = [
  {
    version: appVersion,
    title: 'RaidLedger ersetzt RaidSync',
    changes: [
      'Die Einladungsliste für das Addon ist jetzt für RaidLedger (Knopf „RAID“ im Spiel). Das alte RaidSync-Addon wird nicht mehr gebraucht.',
      'Der Aufstellungs-Import liest den Teilnahme-Export von RaidLedger; Exporte von RaidSync funktionieren weiterhin.',
    ],
  },
  {
    version: '1.0.11',
    title: 'Loot-Import: Entzaubern und Gildenbank',
    changes: [
      'Der Loot-Import zeigt Items, die im Addon entzaubert oder für die Gildenbank behalten wurden. Sie gehören keinem Spieler und werden nicht übernommen.',
    ],
  },
  {
    version: '1.0.10',
    title: 'Wunschliste fürs Addon, Loot-Import mit Würfen',
    changes: [
      'Auf der Charakterseite steht die Wunschliste: Für jedes Item lässt sich Main- oder Second-Spec wählen und bei Main-Spec „Enhanced“ ankreuzen. „Nach oben“ ändert die Reihenfolge.',
      '„Für Addon exportieren“ erzeugt den Text für das Wunschlisten-Addon (ingame /wish import), samt aktuellem EP-Stand. Enhanced würfelt 1+X bis 100+X mit X = EP-Stand, höchstens 50.',
      'Der Loot-Import liest auch Exporte von RaidLedger: Wurf, Spec und Wurfbereich stehen in der Vorschau, Enhanced-Würfe sind als Enhanced vorausgewählt, und Items beim Plündermeister mit bekanntem Gewinner sind für den Gewinner vorausgewählt.',
      'Verwahrte (noch nicht verloste) und im Addon gelöschte Items werden angezeigt, aber nie übernommen. Über bereits übernommene Items entscheidet die Raidleitung per Loot-Korrektur.',
      'Die Herzen im Loot-Katalog zeigen wieder an, welche Items schon auf der Wunschliste stehen, und Wunschlisten sind wieder nach Priorität sortiert.',
    ],
  },
  {
    version: '1.0.9',
    title: 'Absagen: keine EP als Standard, Ersatzbank',
    changes: [
      'Beim Archivieren erhalten abgesagte Spieler standardmäßig keine EP. Mit einem Haken („trotzdem EP geben“) bekommen sie die +10 EP doch.',
      'In der Aufstellung gibt es bei Abgesagten die Schaltfläche „Auf die Bank setzen“.',
    ],
  },
  {
    version: '1.0.8',
    title: 'Raidgesuchtext im Workspace',
    changes: [
      'Der Gesuchtext mit Soll-Besetzung ist direkt beim jeweiligen aktiven Raid erreichbar (für Raidassistenten und höher).',
      'Textvorlage und Soll-Besetzung werden mit „Vorlage speichern“ für die ganze Raidserie gespeichert und sind für alle Raidleiter und Geräte gleich. Im Browser gespeicherte Vorlagen werden einmal übernommen.',
      'Die Aktualisierung alle 15 Sekunden lädt nur noch die Aufstellung und lässt andere Eingaben auf der Raidseite unberührt.',
    ],
  },
  {
    version: '1.0.7',
    title: 'Komfortablere Raidverwaltung, Absagen und EP-Übersicht',
    changes: [
      'Absagen: Spieler können bis zum Raidstart mit „Ich kann nicht kommen“ absagen (optional mit Grund) und mit „Doch dabei“ zurücknehmen. Die Raidleitung sieht Absagen in der Anmeldung und als „Hat abgesagt“ in der Aufstellung und kann sie auch für Spieler eintragen.',
      'Archivieren zeigt vorab eine EP-Übersicht mit aktuellem Stand, Vergabe und Stand danach. Spieler ohne EP stehen mit Grund da, z. B. wegen eines Main-Spec-Items. Abgesagte Spieler lassen sich dort von den EP ausnehmen.',
      'Raid und Loot: Auswahlfelder zeigen Namen statt IDs, der Loot-Empfänger lässt sich per Tippen suchen, bereits vergebene Items sind markiert, Boss und Schwierigkeit bleiben nach dem Vergeben stehen, Enter springt zum nächsten Feld.',
      'Alle „Hinzufügen“-Formulare (Loot, Teilnahme, Raids, Spieler, Serien, Umfragen u. a.) stehen oberhalb der Listen. Neben „Hinzufügen“ steht, was noch fehlt, und leere Listen erklären den Anfang.',
      'Bestätigungen bleiben sichtbar, lassen sich schließen und verschwinden von selbst. „Absagen“, „Archivieren“ und „Loot zurückziehen“ fragen vorher nach.',
      'Anmeldung zum Raid: Charaktere in Klassenfarbe, klarere Hauptaktionen und Hinweise, Hinweis wenn die Anmeldung während der Auswahl schließt. Eigene leere Wunschlisten sind wieder lesbar.',
      'Rechte: Fehlende Berechtigungen werden auf Deutsch gemeldet, Gäste sehen keine Wunschlisten-Schaltflächen, Registrierungsanfragen zeigen Nicht-Admins einen Hinweis.',
      'Weitere Verbesserungen: Spielersuche mit natürlicher Sortierung, lesbarere Anmelde-Reiter, „Jede Woche“ bei Serien, kompaktere Beute-Filter auf dem Handy, Legende im Status-Diagramm und durchgehend deutsche Beschriftungen („Änderungsprotokoll“, „Verlauf“).',
    ],
  },
  {
    version: '1.0.6',
    title: 'Direkte Auswahl bei Teilnahme und Loot',
    changes: [
      'Charaktere und Loot können jetzt in einem gemeinsamen Eingabefeld gesucht und ausgewählt werden.',
    ],
  },
  {
    version: '1.0.4',
    title: 'Aufgeräumte Versionshinweise',
    changes: [
      'Nicht mehr verfügbare Google-Sheets-Funktionen wurden aus der Versionshistorie entfernt.',
    ],
  },
  {
    version: '1.0.3',
    title: 'Google-Sheets-Synchronisierung entfernt',
    changes: [
      'Die Oberfläche für Google-Sheets-Import und -Synchronisierung wurde entfernt.',
    ],
  },
  {
    version: '1.0.2',
    title: 'Zusammenführungen',
    changes: [
      'Doppelte Charaktere und zusammengeführte Spielerkonten werden nach der Übertragung gelöscht.',
    ],
  },
  {
    version: '1.0.1',
    title: 'Registrierungsverwaltung',
    changes: [
      'Registrierungsanfragen sind kompakter, nach aktivem Verlauf getrennt und zeigen Codes erst bei Bedarf.',
      'Doppelte Anfragen für denselben Rising-Gods-Charakter werden verhindert.',
      'Eingelöste Codes zeigen das erstellte oder verwendete Konto.',
    ],
  },
  {
    version: '1.0.0',
    title: 'Erste Version',
    changes: [
      'Raids mit Aufstellung, Beute und Enhance-Points-Verarbeitung verwalten.',
      'Spieler, Charaktere, Umfragen und Raidserien in einer gemeinsamen Gildenverwaltung pflegen.',
      'Historische Raids, Beute und Teilnahme nachvollziehen.',
    ],
  },
]
