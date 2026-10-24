export const appVersion = '1.0.39'

export const changelog = [
  {
    version: appVersion,
    title: 'Loot zurückziehen ohne Begründung im offenen Raid',
    changes: [
      'Loot lässt sich in offenen und geplanten Raids ohne Begründung zurückziehen. Im archivierten Raid bleibt die Begründung Pflicht.',
    ],
  },
  {
    version: '1.0.38',
    title: 'Absage gibt Item-Sperren frei',
    changes: [
      'Wird ein Raid abgesagt, laufen seine noch nicht eingelösten Item-Sperren jetzt ab. Dabei gehen keine Enhance Points verloren. Sperren früher abgesagter Raids bleiben unverändert.',
    ],
  },
  {
    version: '1.0.37',
    title: 'Übersichtlichere Anmeldungen und Umfragen bearbeiten',
    changes: [
      'Die Anmeldeliste eines Raids zeigt jeden Spieler in einer Zeile, offene Anmeldungen zuerst, mit Klassensymbol, Klasse und Spec. Oben steht, wie viele ausstehend, eingetragen, unsicher und abgesagt sind.',
      'Die Raidleitung sieht die Anmeldeliste ganz oben; die eigene Anmeldung ist darunter eingeklappt.',
      'Trägt die Raidleitung deinen Charakter direkt in die Aufstellung ein, zeigt „Meine nächsten Raids“ dich jetzt als bestätigt.',
      'Die Raidleitung kann Umfragen nachträglich bearbeiten: Titel, Beschreibung, Ende, Antwortoptionen, und geschlossene Umfragen wieder öffnen. Umbenannte Optionen behalten ihre Stimmen.',
    ],
  },
  {
    version: '1.0.36',
    title: 'RaidWishlist 2.1.0',
    changes: [
      'Neue RaidWishlist-Version zum Herunterladen unter „Addons“. Das Fenster lässt sich an der Ecke unten rechts größer ziehen, die Listen zeigen dann mehr Zeilen. Mit – und + unten im Fenster (oder /wish scale) wird es kleiner oder größer, von 60 bis 150 %. /wish reset stellt Größe und Position zurück.',
      'Ein Item, das du im letzten Raid bekommen hast, verhindert im nächsten Raid nicht mehr den Würfel-Hinweis.',
      'Items, die dein Spiel noch nie gesehen hat, zeigen kurz „[Gegenstand 12345]“ und dann ihren Namen statt „item:12345“.',
    ],
  },
  {
    version: '1.0.35',
    title: 'Andere Geräte nach Passwortänderung abgemeldet',
    changes: [
      'Ändert jemand sein Passwort, werden alle anderen Geräte abgemeldet; das aktuelle Gerät bleibt angemeldet.',
      'Ändert ein Admin Passwort, Benutzernamen oder Rolle eines Spielers, wird dieser Spieler überall abgemeldet.',
    ],
  },
  {
    version: '1.0.34',
    title: 'Fehlermeldungen werden vorgelesen',
    changes: [
      'Alle Fehlermeldungen nutzen jetzt dieselbe Darstellung und werden von Screenreadern angesagt.',
    ],
  },
  {
    version: '1.0.33',
    title: 'Aktueller Raid führt zum nächsten Termin',
    changes: [
      '„Aktueller Raid“ öffnet einen noch laufenden Raid oder den nächsten anstehenden Termin. Abgeschlossene, abgesagte und archivierte Raids werden nicht mehr als aktuell angezeigt.',
    ],
  },
  {
    version: '1.0.32',
    title: 'RaidLedger 2.5.1',
    changes: [
      'Neue RaidLedger-Version zum Herunterladen unter „Addons“. Behält der Plündermeister ein Item für den Gewinner eines Wurfs, nennt die Ansage im Raid jetzt den Gewinner („geht an Alice – Main-Spec (bei Tester bis zum Handel)“) statt den Plündermeister. Für den Gewinner gezählt wurde es schon vorher sofort.',
    ],
  },
  {
    version: '1.0.31',
    title: 'Höchstens fünf falsche Codes',
    changes: [
      'Bei der Anmeldung mit zweitem Faktor sind höchstens fünf falsche Codes erlaubt. Danach meldest du dich erneut mit deinem Passwort an.',
      'Ein falscher Code wird jetzt auf Deutsch gemeldet.',
    ],
  },
  {
    version: '1.0.30',
    title: 'Schutz vor Passwort-Raten',
    changes: [
      'Nach 30 fehlgeschlagenen Anmeldungen von derselben Internetadresse innerhalb von 15 Minuten sind weitere Anmeldeversuche von dort vorübergehend gesperrt, egal für welchen Benutzernamen. Erfolgreiche Anmeldungen zählen nicht mit.',
    ],
  },
  {
    version: '1.0.29',
    title: 'Admins brauchen einen zweiten Faktor',
    changes: [
      'Admin-Funktionen gehen nur noch mit Authenticator-App oder Sicherheitsschlüssel. Admins ohne zweiten Faktor landen nach dem Anmelden im Reiter „Konto“ und sehen oben einen Hinweis, der sich nicht ausblenden lässt.',
      'Für alle anderen Rollen bleibt der zweite Faktor freiwillig.',
      'Behoben: Wer nur einen Sicherheitsschlüssel und Wiederherstellungscodes hat, kann sich wieder mit einem Wiederherstellungscode anmelden.',
    ],
  },
  {
    version: '1.0.28',
    title: 'Hinweis zur Kontosicherheit',
    changes: [
      'Wer noch keinen zweiten Faktor (Authenticator-App oder Sicherheitsschlüssel) eingerichtet hat, sieht oben einen schmalen Hinweis. Er verschwindet, sobald einer eingerichtet ist, und lässt sich für die laufende Sitzung ausblenden.',
      'Ein Klick führt zur neuen Seite „Kontosicherheit“: Sie erklärt mit Zahlen von Microsoft, Google, Verizon und dem BSI, warum ein zweiter Faktor so viel bringt, und wie du ihn in zwei Minuten einrichtest. Alle Quellen sind verlinkt.',
      'Von dort führt „Jetzt einrichten“ direkt in den Reiter „Konto“ deines Profils.',
    ],
  },
  {
    version: '1.0.27',
    title: 'RaidLedger 2.5.0 und RaidWishlist 2.0.0',
    changes: [
      'Neue Addon-Versionen zum Herunterladen unter „Addons“. RaidLedger hat jetzt fünf Reiter (Beute, Vergaben, Spieler, Raid, Einstellungen): alles geht per Klick, jeder Knopf hat einen Tooltip.',
      'RaidLedger: Unter „Spieler“ siehst du Items und Tokens pro Spieler und korrigierst sie mit − / +. Unter „Einstellungen“ legst du fest, wer außer dem Plündermeister etwas ändern darf, und siehst, wer welches Addon hat. Gleiche Tokens einer Leiche werden in einem Wurf verteilt, Gruppenplündern wird mitgeschrieben.',
      'RaidWishlist: drei Reiter (Wunschliste, Offen, Dieser Raid), jede Liste scrollt jetzt mit dem Mausrad bis zum letzten Eintrag.',
    ],
  },
  {
    version: '1.0.26',
    title: 'Charakter dem richtigen Spieler zuordnen',
    changes: [
      'Admins können unter Spieler → Charaktere einen versehentlich falsch zugeordneten Charakter zu einem anderen Spieler verschieben. Die Raid-Aufstellungen zeigen danach den richtigen Spieler; Loot und EP bleiben unverändert. Die Änderung steht im Audit-Protokoll.',
    ],
  },
  {
    version: '1.0.25',
    title: 'Aktueller Raid, Boss-Reihenfolge und Beute ohne Empfänger',
    changes: [
      'Neu im Menü: „Aktueller Raid“ (mobil „Aktuell“) führt direkt zum Raid von heute Abend, also zum neuesten Raid, der schon begonnen hat (sonst zum nächsten).',
      'Die Bosse stehen jetzt in einer Reihenfolge, die ein Admin unten auf der Seite „Beute“ festlegt: bei der Boss-Auswahl beim Loot-Eintragen und bei den Boss-Gruppen in der Beuteliste eines Raids.',
      'Neu im Raid unter „Loot“: „Beute ohne Empfänger“ für Items, die entzaubert wurden, in die Gildenbank gehen oder die niemand wollte. Sie zählen für keinen Spieler. Der Addon-Import übernimmt entzauberte und Gildenbank-Items von RaidLedger jetzt dorthin.',
    ],
  },
  {
    version: appVersion,
    title: 'Schattengram im Raid: freie Auswahl aus der Aufstellung',
    changes: [
      'Bei „Schattengram im Raid“ kann die Raidleitung jetzt auch Krieger, Paladine und Todesritter aus der Aufstellung einteilen, die nicht auf der Schattengram-Liste stehen.',
      'Von der Liste werden pro Schritt nur noch Charaktere angeboten, die gerade bei diesem Schritt sind. Wer schon Splitter sammelt, taucht zum Beispiel nicht mehr bei Blut auf.',
    ],
  },
  {
    version: '1.0.23',
    title: 'Schattengram: Questreihe und Einteilung im Raid',
    changes: [
      'Neu im Menü: „Schattengram“. Die Liste zeigt, wer an Schattengram arbeitet, in welcher Reihenfolge und bei welchem Schritt (Vorquest, Seelen, Seuche, Blut, Frost, Splitter, Abschluss). Mehrere Leute können gleichzeitig dran sein.',
      'Im Raid unter „Aufstellung“ steht jetzt „Schattengram im Raid“: wer Seuche, Blut, Frost und die Splitter macht. Vorgeschlagen wird der höchste Listenplatz aus der Aufstellung; die Raidleitung kann das pro Raid ändern.',
      'Für die Raidleitung: Splitter mit +1 / −1 zählen, das Ziel ist 50 und bleibt änderbar, Charaktere auf einen bestimmten Platz setzen, falsche Einträge entfernen. Nur Krieger, Paladine und Todesritter können eingetragen werden.',
    ],
  },
  {
    version: '1.0.22',
    title: 'Addons zum Herunterladen',
    changes: [
      'Neu im Menü: „Addons“. RaidLedger (für Raidleitung und Plündermeister) und RaidWishlist (für jeden Raider) gibt es dort direkt als ZIP, mit Installation, Bildern aus dem Spiel, allen Befehlen und wie sie mit der Website zusammenarbeiten.',
      'Die Seiten sind auch ohne Login erreichbar.',
      'Die „Einladungen für Addon“ enthalten bei Raids aus einer Raidserie jetzt Datum und Startzeit. Damit legt RaidLedger mit „KALENDER-TERMIN“ den Raid im Spielkalender an und lädt alle ein.',
    ],
  },
  {
    version: '1.0.21',
    title: 'Wer hat schon ein Konto?',
    changes: [
      'Admins sehen unter „Spieler“ zu jedem Spieler seine Charaktere, sein Konto und seine Rolle, dazu oben, wie viele Spieler schon ein Konto haben.',
      'Beim Konto steht der Login-Name und ob die Person sich mit einem Code registriert hat (mit Datum und Charakter) oder ob ein Admin den Login angelegt hat. Mit dem Filter „Ohne Konto“ siehst du, wer sich noch registrieren muss.',
      'Die Suche findet für Admins auch Charakter- und Login-Namen.',
      'Unter „Registrierung“ → „Verlauf“ zeigt die neue Spalte „Verknüpfter Spieler“, für welchen Spieler ein Code eingelöst wurde. Die Status stehen jetzt auf Deutsch.',
    ],
  },
  {
    version: '1.0.20',
    title: 'Anleitung auf der Website',
    changes: [
      'Neu im Menü: „Anleitung“. Sie erklärt Schritt für Schritt mit Bildern und Beispielen, wie alles funktioniert: Konto bekommen, anmelden und absagen, Wunschliste und Addon-Export, EP und Item-Locks, für die Raidleitung Raids planen, Aufstellung, Gesuchtext und Loot, für Admins die Verwaltung.',
      'Oben wählst du deinen Bereich, dein eigener ist markiert. Am Ende stehen häufige Fragen.',
      'Die Anleitung ist auch ohne Login erreichbar, damit neue Leute schon vor der Registrierung nachlesen können. Auf der Login-Seite führt „Neu hier?“ direkt dorthin.',
    ],
  },
  {
    version: '1.0.19',
    title: 'Unsicher als Rückmeldung',
    changes: [
      'Neben „Kann nicht kommen“ gibt es jetzt „Unsicher“, optional mit Hinweis wie „komme evtl. später“. Mit „Sicher dabei“ nimmst du es zurück.',
      'Die Raidleitung sieht Unsichere unter „Absagen & Unsicher“ und in der Aufstellung und kann Spieler selbst als unsicher markieren.',
      'Unsicher hat keinen Einfluss auf EP: Nur Abgesagte können beim Archivieren von den EP ausgenommen werden.',
    ],
  },
  {
    version: '1.0.18',
    title: 'Einfacher anmelden, absagen und Wunschliste exportieren',
    changes: [
      'Auf der Übersicht zeigt „Meine nächsten Raids“, ob du angemeldet bist. Mit einem Klick meldest du dich mit deinem Main an, sagst mit „Kann nicht kommen“ (optional mit Grund) ab oder nimmst die Absage mit „Doch dabei“ zurück.',
      'Raids öffnen sich für Mitglieder direkt bei den Anmeldungen und zeigen deinen Status.',
      'Neu im Menü: „Meine Wunschliste“. Oben steht, wie die Liste ins Addon kommt, und „Export kopieren“ kopiert sie mit einem Klick.',
      'Im Beute-Katalog bestätigt ein Hinweis jedes hinzugefügte Item und führt direkt zur Wunschliste.',
      'Nach der Anmeldung über „Anmelden“ landest du auf der Übersicht statt auf „Seite nicht gefunden“.',
    ],
  },
  {
    version: '1.0.17',
    title: 'Kurzer Gesuchtext',
    changes: [
      'Neuer Platzhalter {{SETUP_KURZ}} im Gesuchtext: fasst die offenen Plätze kurz zusammen, z. B. „2 Heal (Baum, Diszi), 2 MDD (beliebig), 2 RDD (Mage, SP)“.',
      'Mit {{TANK_SPECS}}, {{HEAL_SPECS}}, {{MDD_SPECS}} und {{RDD_SPECS}} lassen sich die gesuchten Specs einer Rolle in eigene Sätze einbauen. {{SETUP}} bleibt unverändert.',
    ],
  },
  {
    version: '1.0.16',
    title: 'Raids im Voraus planen',
    changes: [
      'Auf der Raidseite legt „Termine im Voraus anlegen“ alle Raids einer Serie bis zu einem gewählten Tag an. Die Anmeldung ist sofort offen, sodass sich alle schon Wochen vorher anmelden können.',
      'Bei jeder Raidserie lässt sich einstellen, wie viele Wochen automatisch im Voraus geplant bleiben. Aufgefüllt wird beim Speichern der Serie und bei jedem Archivieren.',
      'Abgesagte Termine werden beim Archivieren übersprungen; der nächste Termin der Serie wird zum Nachfolger.',
    ],
  },
  {
    version: '1.0.15',
    title: 'Audit-Protokoll zeigt alle Änderungen',
    changes: [
      'Das Audit-Protokoll zeigt jetzt auch alltägliche Änderungen: Charaktere in die Aufstellung aufnehmen, Raids anlegen, planen, absagen und archivieren, Wunschlisten, Umfragen und Stimmen, Item-Locks, Raidserien, Gildenränge und Raidgruppen, neue Spieler, Zugänge, Passwort- und Zwei-Faktor-Änderungen sowie Registrierungen.',
      'Passwörter, Codes und andere Geheimnisse werden dabei nie gespeichert.',
    ],
  },
  {
    version: '1.0.14',
    title: 'Teilnehmer aus dem Raid entfernen',
    changes: [
      'In der Aufstellung geplanter und offener Raids lässt sich ein Charakter über „Bearbeiten“ mit „Aus dem Raid entfernen“ wieder herausnehmen. Hat er in diesem Raid schon Loot erhalten, muss der Loot zuerst zurückgezogen werden.',
    ],
  },
  {
    version: '1.0.13',
    title: 'Lesbares Audit-Protokoll',
    changes: [
      'Das Audit-Protokoll fasst Änderungen als verständliche Ereignisse mit Namen, deutschen Aktionen und Feldvergleich zusammen. Technische Rohdaten bleiben bei Bedarf aufklappbar.',
    ],
  },
  {
    version: '1.0.12',
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
