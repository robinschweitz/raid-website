export const appVersion = '1.0.8'

export const changelog = [
  {
    version: appVersion,
    title: 'Raidgesuchtext im Workspace',
    changes: [
      'Der Gesuchtext mit Soll-Besetzung ist direkt beim jeweiligen aktiven Raid erreichbar.',
    ],
  },
  {
    version: appVersion,
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
