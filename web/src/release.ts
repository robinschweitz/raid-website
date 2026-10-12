export const appVersion = '1.0.2'

export const changelog = [
  {
    version: appVersion,
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
