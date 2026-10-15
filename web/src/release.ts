export const appVersion = '1.0.14'

export const changelog = [
  {
    version: appVersion,
    title: 'Spielersuche und feinere Details',
    changes: [
      'Spielerliste: Suchfeld mit Trefferanzahl und natürliche Sortierung (Spieler2 vor Spieler10).',
      'Raidserien: „Jede Woche“ statt „Alle 1 Wochen“.',
      'Beute: Die Filter passen auf dem Handy in zwei Spalten und brauchen weniger Platz.',
      'Raid: Der Bereich „Planung & Details“ heißt auch in seiner Überschrift so.',
    ],
  },
  {
    version: '1.0.13',
    title: 'Sicherheitsabfragen und einheitliche Texte',
    changes: [
      '„Absagen“, „Archivieren“ und „Loot zurückziehen“ fragen vor dem Ausführen nach.',
      'Neben „Hinzufügen“ steht, was noch fehlt (Charakter wählen, Begründung eintragen).',
      'Statistiken: Legende mit Anzahl beim Status-Diagramm, durchgehend deutsche Beschriftungen („Einträge“, „Vergabemethode“). Übersicht und Profil: „Raidaufstellung“ und „Verlauf“ statt englischer Titel.',
      'Auf dem Handy steht in der Anwesenheitskarte wieder ein Abstand zwischen Beschriftung und Zahl.',
    ],
  },
  {
    version: '1.0.12',
    title: 'Alle Hinzufügen-Formulare oben',
    changes: [
      '„Neue Raidserie“, „Neue Umfrage“, „Gildenrang hinzufügen“, „Raidgruppe hinzufügen“ sowie „Charakter hinzufügen/beanspruchen“ stehen jetzt oberhalb der Listen.',
      'Nach „Hinzufügen“ bleibt die Teilnahme-Eingabe offen und der Cursor steht wieder im Charakterfeld, damit sich mehrere Charaktere hintereinander eintragen lassen.',
      'Leere Listen erklären, womit man beginnt (Loot, Raids, Umfragen, Raidserien).',
    ],
  },
  {
    version: '1.0.11',
    title: 'Formulare zuerst, schnellere Lootvergabe',
    changes: [
      '„Loot erfassen“, „Teilnahme hinzufügen“, „Neuer Raid“ und „Neuer Spieler“ stehen jetzt oberhalb der Listen statt ganz unten.',
      'Der Empfänger lässt sich wie Item und Charakter per Tippen suchen.',
      'Neben „Vergeben“ steht, was noch fehlt. Bereits vergebene Items sind in der Auswahl mit „bereits n× vergeben“ markiert.',
      'Mit Enter im Item-Feld springt der Cursor zum Empfänger. Bestätigungen verschwinden nach einigen Sekunden von selbst.',
    ],
  },
  {
    version: '1.0.10',
    title: 'Mehrere Items hintereinander vergeben',
    changes: [
      'Nach dem Vergeben bleiben Boss und Schwierigkeit ausgewählt, damit das nächste Item vom selben Boss direkt gewählt werden kann.',
    ],
  },
  {
    version: '1.0.9',
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
