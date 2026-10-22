# Coding Rules

Regeln für Code in diesem Repository: die Code-Konventionen für den Alltag des Schreibens.
`AGENTS.md` importiert dieses Dokument.

Zuständigkeit: Verhalten steht in `REQUIREMENTS.md`, Gestaltung in `DESIGN.md`, Aufbau und
Abhängigkeitsregeln in `ARCHITECTURE.md`, Sicherheitsregeln in `SECURITY.md`, die Arbeitsweise in
`AGENTS.md`. Dieses Dokument wiederholt deren Inhalte nicht, sondern verweist auf ihre IDs.
`docs/decisions/*.md` ist die Entscheidungshistorie.
Widersprüche, Rückfragen und ADRs: Arbeitsregeln 1 bis 3 in `AGENTS.md`.

Die MUSS-Regeln sind nicht verhandelbar.

---

## 1. Grundsätze

1.1 **Langweilig, explizit, vorhersagbar.** Keine cleveren Abstraktionen, kein verstecktes
Verhalten, keine spekulative Generizität. Lieber lang und offensichtlich als kurz und klug.

1.2 **Domain-first.** DR-6.

1.3 **Repository-Besitz.** SQL und Bun-Query-Builder erscheinen ausschließlich in
`internal/<domain>/repo.go`; Modelle dürfen Bun-Struct-Tags tragen (`ARCHITECTURE.md` DR-4,
ADR-002).

1.4 **Nicht raten.** `AGENTS.md` Arbeitsregel 1.

1.5 **Nie still überschreiben.** Massenwrites erzeugen einen Report (`REQUIREMENTS.md` FR-31.3).

1.6 **Archiv statt Löschen.** Siehe FR-31.1 und die abschließende Liste der Hard Deletes in FR-31.2.

1.7 **Änderungen sind auditiert.** Jeder Write schreibt seinen Audit-Eintrag in derselben
Transaktion (FR-26.1, FR-26.2, DR-5); Reason-Pflicht nach FR-26.5. Zahlenkorrekturen sind neue
Ausgleichstransaktionen im Ledger, kein `UPDATE` auf einem Saldo (FR-16.1).

1.8 **UTC speichern, lokal anzeigen.** `ARCHITECTURE.md` „Persistence“, FR-28.4.

1.9 **Autorisierung ist serverseitig.** `SECURITY.md` SC-001 bis SC-003.

1.10 **Konfiguration ist explizit.** Admin-änderbare Werte sind DB-Zeilen, keine Konstanten oder
Migrationen (FR-8.1, FR-9.2); Umgebung nur über `internal/config` (DR-7).

1.11 **Fehler sind explizit.** Domain-Pakete liefern typisierte Fehler; Handler bilden sie auf
HTTP-Status und einen stabilen, maschinenlesbaren `code` ab. Kein stilles `nil`-Schlucken.

1.12 **Neues Verhalten braucht Tests.** Definition of Done in `AGENTS.md`.

1.13 **Idiomatisches Go.** Die Konventionen aus *Effective Go* und der Standard Library sind der
Massstab. Eine Dependency, deren Aufgabe die Standard Library bereits erledigt, wird nicht
eingeführt; jede neue Dependency braucht die Freigabe des Owners (DR-11).

---

## 2. Gates

Die Gates und ihre Befehle stehen in `AGENTS.md` „Commands“. Formatierungsfehler korrigiert
`gofmt -w cmd internal`.

`golangci-lint` nutzt ausschließlich die Linter aus `.golangci.yml`: `gofmt`, `govet`,
`errcheck`, `staticcheck`, `gosimple`, `ineffassign`, `unused`. Keine Linter hinzufügen oder
entfernen, ohne das im Repo zu belegen. Ist das Binary nicht installiert, wird das gemeldet —
nicht stillschweigend übersprungen.

---

## 3. Formatierung

3.1 `gofmt` ist die einzige Formatierungsquelle. Kein manueller Stil.

3.2 Es gibt keine Diskussion über Tabs vs. Spaces in Go. `gofmt` entscheidet; das Repo steht auf
Tabs und wird nicht umgestellt.

3.3 Keine Formatierung, die der Compiler nicht versteht, und keine gesonderten Regeln für einzelne
Dateien.

3.4 Lange Ausdrücke werden nicht manuell umgebrochen. Zusammengesetzte Literale oder
SQL-Statements über mehrere Zeilen sind normal und werden von `gofmt` stabil gehalten.

---

## 4. Benennung

4.1 Namen tragen Bedeutung. Keine Platzhalter.

Gut:

```go
raidID
playerID
attendance
pointBalance
lootAward
scheduledStart
```

Schlecht:

```go
data
tmp
foo
val
thing
obj
stuff
```

4.2 Kurze lokale Variablen sind erlaubt, wenn die Bedeutung sofort klar ist:

```go
i
err
tx
db
r
w
```

4.3 Boolesche Felder und Variablen beginnen mit `is`, `has`, `can` oder `enable`.

4.4 IDs heißen `<entity>ID`, Zeitstempel benennen, was sie sind (`scheduledStart`, `completedAt`,
`archivedAt`). `start` allein ist zu unscharf.

4.5 `Get`, `List`, `Create`, `Update`, `Exists` sind die Namen der Repository-Methoden.
Services benennen die Handlung: `Award`, `CompleteRaid`, `Roll`, `Lock`.

4.6 `manager`, `helper`, `util` als Bezeichner sind abgelehnt.

---

## 5. Packages und Verantwortung

5.1 Package-Namen: kurz, lowercase, singular, wenn sinnvoll, keine unnötigen Abkürzungen, keine
Wiederholung des Package-Namens im Bezeichner.

Gut:

```go
raid.Service
player.Repo
loot.Award
```

Schlecht:

```go
raid.RaidService
player.PlayerRepository
loot.LootAwardObject
```

5.2 Der Package-Name muss beantworten, was hineingehört und was nicht. Bei zu vielen
Verantwortlichkeiten wird aufgeteilt — aber ohne vorschnelle Mikro-Packages.

5.3 Keine Samml Packages, die fachlich Fremdes bündeln:

```text
utils/
helpers/
common/
misc/
```

Fachcode liegt beim Fach:

```text
nicht:   internal/utils/calculate_points.go
sondern:  internal/enhancepoints/completion.go
           internal/loot/tokens.go
```

---

## 6. Schichten und Abhängigkeiten

6.1 Import-Richtung und Schichten: `ARCHITECTURE.md` DR-1, DR-2 und DR-6.

6.2 Jede Domain mit Repository erhält `NewRepo(db bun.IDB)` und `WithTx(tx bun.Tx)`;
Multi-Table-Writes laufen in `BeginTx` und werden über `WithTx` gereicht (DR-5).

6.3 Der Actor kommt aus dem Kontext der Middleware (`internal/httpapi/v1/actor.go`, SC-003).

---

## 7. Fehlerbehandlung

7.1 Typisierte Fehler auf Package-Ebene:

```go
var ErrLockNotAllowed = errors.New("lock not allowed")
```

Handler prüfen mit `errors.Is` und bilden auf `WriteError` ab.

7.2 Repositories und Services wrappen mit Kontext und `%w`:

```go
return fmt.Errorf("load raid %d: %w", raidID, err)
```

7.3 Fehler werden nicht verschluckt. `errcheck` ist aktiv. Wo Ignorieren gewollt ist, ist es
explizit und begründet:

```go
defer func() { _ = tx.Rollback() }()
```

7.4 `panic` ist in Domain-, Repository- und HTTP-Code verboten. Zulässig ist es nur in `main`,
wenn der Prozess ohnehin nicht weiterlaufen kann.

7.5 Kein leeres `error` als Signal, kein `return nil` bei einem Fehlerfall.

---

## 8. Kontext

8.1 `ctx context.Context` ist immer der erste Parameter und wird bis in die Repositories
durchgereicht.

8.2 Request-gebundene Werte werden ausschließlich über den Kontext transportiert.

8.3 Nebenläufigkeit nur, wenn sie nötig ist. Wenn doch, mit `sync.WaitGroup` und
Fehlersammlung aus der Standard Library — kein `go` ohne Synchronisation, keine neue
Concurrency-Dependency. Reihenfolge ist fast immer die einfachere und korrektere Wahl.

---

## 9. Datenbank und Migrationen

9.1 Migrationen sind nummerierte Datei-Paare in `internal/migrations` (Dateinamen-Regex
`^(\d{1,14})_([0-9a-z_\-]+)\.`):

```text
001_init.up.sql
001_init.down.sql
```

Eingebettet per `go:embed`, Reihenfolge wird nur ergänzt.

9.2 Bereits angewandte Migrationen werden nicht geändert. Eine Korrektur ist eine neue Datei.

9.3 Schema mit Indizes und Defaults definieren; Zeitstempel als UTC (`ARCHITECTURE.md` „Persistence“).

9.4 Queries: `SECURITY.md` SC-021 und DR-4.

---

## 10. JSON und API (Go-Seitig)

API-Vertrag (Envelope, `snake_case`, RFC3339, `[]` statt `null`, Versionierung):
`ARCHITECTURE.md` „API style“. Strenges Decoding: SC-020. Umsetzung im Code:

10.1 DTOs liegen in `internal/httpapi/v1` und decken exakt die akzeptierten Felder ab.

10.2 Listen über `writeListJSON` bzw. `nonNil` schreiben, nie rohes `WriteJSON` auf Slices;
abgesichert durch `TestListEndpointsReturnEmptyArrays`.

10.3 Fehler über `WriteError` mit stabilem `code` (SC-023).

---

## 11. Konfiguration und Abhängigkeiten

11.1 Konfiguration und Dependencies: DR-7, DR-11 und SC-033; Admin-änderbare Werte siehe §1.10.
Default-Zugangsdaten sind nur für die Entwicklung erlaubt.

---

## 12. Kommentare und Dokumentation

12.1 Kommentare und Doc-Strings auf Englisch.

12.2 Nutzertexte auf Deutsch (FR-28.3), auch Item-Namen und Hinweise an Nutzer.

12.3 Ein Kommentar erklärt, warum etwas so gemacht ist, nicht was die Zeile tut. Offensichtlicher
Code bleibt ohne Kommentar.

12.4 Exported Symbole bekommen einen Doc-Kommentar, der mit dem Namen beginnt.

12.5 Keine auskommentierten Blöcke — die Git-Historie ist der Ort dafür. Keine `TODO` ohne die
Frage, wie sie aufgelöst wird.

---

## 13. Tests

13.1 Tests liegen als `*_test.go` im selben Package.

13.2 Getestet werden Verhalten **und** Berechtigungsgrenze: jede neue Mutation bekommt einen Fall,
der eine zu niedrige Rolle mit 403 belegt.

13.3 Jeder Test hat eine isolierte Test-DB (eigener Dateiname, kein gemeinsames `raid.db`),
siehe `internal/httpapi/api_test.go` und `internal/raidcompletion/service_test.go`.

13.4 Table-driven, wo es die Fälle klarer macht; sonst ein Test pro Fall mit sprechendem Namen.

13.5 Kein `time.Sleep` in Tests. Zeit wird über injizierte Werte oder das Prüfen von
`CreatedAt`/`UpdatedAt` bestimmt.

13.6 Keine externen Dienste, kein Netzzugriff, keine Abhängigkeit von der Ausführungsreihenfolge
zwischen `go test`-Paketen.

13.7 Ein Bug wird zuerst als Regressionstest festgehalten, dann behoben.

---

## 14. Frontend

14.1 TypeScript strict. Kein `any`, außer an bewusst dokumentanten Grenzen. `web/src/types.ts`
ist die Quelle der API-Typen.

14.2 API-Aufrufe nur über `web/src/api.ts` (DR-10); Autorisierung bleibt beim Server (SC-001);
keine neue Router-, State- oder HTTP-Library (DR-11).

14.3 Seiten liegen unter `web/src/pages`, gemeinsame Infrastruktur in `api.ts`, `auth.tsx`,
`router.tsx`. Nutzertexte auf Deutsch (FR-28.3).

14.4 `npm run lint` muss laufen. Die bekannten React-Fast-Refresh-Warnungen werden nicht
wegkonfiguriert; sie weisen auf Komponenten hin, die nicht in eigene Dateien gehören.

---

## 15. Git und Änderungen

15.1 Eine fachliche Änderung pro Commit, Commits klein, Messages im Stil der Historie.

15.2 Keine Secrets im Repository (SC-033).

15.3 ADRs: `AGENTS.md` Arbeitsregel 3.

15.4 Kein Force-Push, kein Rewrite der Historie ohne ausdrückliche Freigabe.

---

## 16. Wenn etwas unklar ist

Eine Regel, die weder hier noch in den Spezifikationsdateien steht, ist keine bekannte Regel.
Vorgehen: `AGENTS.md` Arbeitsregeln 1 und 2 (fragen, dann in der zuständigen Datei festhalten).
