export const appVersion = '1.0.11'

export const changelog = [
  {
    version: appVersion,
    title: 'Manuelle Sheet-Auswahl',
    changes: [
      'Die manuelle Synchronisierung startet ohne vorausgefüllte Sheet-ID; automatische Synchronisierungen bleiben über ihre gespeicherten Zuordnungen aktiv.',
    ],
  },
  {
    version: '1.0.10',
    title: 'Aktives Synchronisierungs-Sheet',
    changes: [
      'Die Synchronisierung zeigt standardmäßig das aktuell verknüpfte Google Sheet statt der historischen Importquelle.',
    ],
  },
  {
    version: '1.0.9',
    title: 'Automatische Google-Sheets-Synchronisierung',
    changes: [
      'Verknüpfte Google Sheets werden jede Minute in beide Richtungen synchronisiert; fehlerhafte Vorschauen werden nur protokolliert.',
    ],
  },
  {
    version: '1.0.8',
    title: 'Sheet-Verknüpfungen im Raid',
    changes: [
      'Raid-Details zeigen nun auch Verknüpfungen, die durch die historische Synchronisierungsübernahme angelegt wurden.',
    ],
  },
  {
    version: '1.0.7',
    title: 'Wöchentliche Raid-Finalisierung',
    changes: [
      'Synchronisierte Raid-Archive finalisieren den aktuellen Raid mit EP-Verarbeitung und übernehmen die neue ICC25-Aufstellung in den Nachfolger.',
    ],
  },
  {
    version: '1.0.6',
    title: 'Zusammengeführte Sheet-Identitäten',
    changes: [
      'Die historische Sheet-Zuordnung erkennt Charaktere weiter über die Alias-Historie nach einer Zusammenführung.',
    ],
  },
  {
    version: '1.0.5',
    title: 'Historische Sheet-Zuordnung',
    changes: [
      'Bestehende historische Raids können nach einer Vorschau mit einem Google Sheet verknüpft werden.',
      'Mehrdeutige Spieler- und Charakterbesitzer werden vor der auditierten Übernahme ausdrücklich ausgewählt.',
    ],
  },
  {
    version: '1.0.4',
    title: 'Google-Sheets-Synchronisierung',
    changes: [
      'Die Synchronisierung prüft eingehende Änderungen zuerst als Vorschau und überträgt ausstehende Website-Änderungen vor dem Anwenden.',
      'Historische Importe bleiben als schreibgeschützte Historie sichtbar; ausgehende Übertragungen können separat erneut gesendet werden.',
    ],
  },
  {
    version: '1.0.3',
    title: 'Google-Sheet-Verknüpfungen',
    changes: [
      'Raid-Details zeigen die verknüpfte Google-Sheet-Quelle; Admins können Sheet und Raid-Schlüssel mit Begründung hinterlegen.',
      'Loot aus verknüpften Raids kann in beide Richtungen auditiert synchronisiert und bei Übertragungsfehlern erneut gesendet werden.',
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
      'Historische Raids, Beute und Teilnahme nachvollziehen sowie Legacy-Daten importieren.',
    ],
  },
]
