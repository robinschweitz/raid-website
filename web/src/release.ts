export const appVersion = '1.0.9'

export const changelog = [
  {
    version: appVersion,
    title: 'Auswahl ohne interne IDs',
    changes: [
      'Charakter- und Item-Vorschläge zeigen nur noch Namen. Eine ID erscheint nur, wenn zwei Einträge sonst nicht zu unterscheiden wären.',
      'Eine genau eingegebene Item-ID wählt weiterhin das passende Item aus.',
      '„Entfernen“ steht direkt neben dem Auswahlfeld statt in einer zusätzlichen Zeile.',
    ],
  },
  {
    version: '1.0.8',
    title: 'Übersichtlichere Raidverwaltung',
    changes: [
      'Charakter- und Item-Auswahl nutzen die volle Breite, die Auswahl bleibt vollständig lesbar.',
      'Bestätigungen bleiben sichtbar, lassen sich schließen und verschwinden beim Wechsel des Reiters.',
      'Gefilterte Aufstellungen zeigen „Keine Treffer“ statt „Noch nicht besetzt“; „Filter zurücksetzen“ steht direkt bei der Trefferanzahl.',
      'Die Raidbereiche sind auf dem Handy ein 2×2-Raster statt einer abgeschnittenen Leiste. Die Rolle der Teilnahme hat jetzt eine Beschriftung.',
    ],
  },
  {
    version: '1.0.7',
    title: 'Schnellere Raidverwaltung',
    changes: [
      'Auswahlfelder zeigen die getroffene Auswahl, erfolgreiche Änderungen werden bestätigt und die Raidaufstellung kann gefiltert werden.',
      'Teilnahme- und Loot-Editoren sind auf kleinen Bildschirmen besser bedienbar.',
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
