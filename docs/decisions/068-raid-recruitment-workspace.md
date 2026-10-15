# ADR 068: Raidgebundener Gesuchtext

Status: Accepted

## Decision

Der Gesuchtext gehört zum Workspace des jeweiligen aktiven Raids und ist kein
eigenständiger Navigationsbereich. Vorlage und Soll-Besetzung bleiben pro Raid im lokalen
Browser-Speicher abgelegt. Die Lückenberechnung verwendet ausschließlich die gespeicherte
Aufstellung dieses Raids und aktualisiert sie regelmäßig.

## Consequences

Der Gesuchtext ist im Tab `Gesuchtext` eines geplanten oder offenen Raids erreichbar.
Abgeschlossene und abgesagte Raids bieten keine Rekrutierungskonfiguration.
