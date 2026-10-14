# ADR 060 — Google-Sheets-Sync im Adminbereich konfigurieren

Status: Angenommen (Owner-Anforderung vom 14.10.2026; ergänzt ADR 057–059).

## Kontext und Entscheidung

Der Eigentümer möchte die Sheet-Sync-Konfiguration in der GUI bearbeiten, statt
vor jedem Start Umgebungsvariablen eintragen zu müssen. Unter **Importe**
können Administratoren Aktivierung, automatischen Zeitplan, Intervall,
Archivbegründung und das Google-Servicekonto einstellen. Änderungen werden
atomar in der SQLite-Datenbank gespeichert und ohne Neustart wirksam.
Die Admin-API bietet GET/PUT `/api/v1/sheet-sync/config`.
Die bestehende Env-Konfiguration bleibt lediglich als Bootstrap/Fallback erhalten;
eine gespeicherte GUI-Konfiguration geht vor. Login-/Datenbank-/Server- und
Proxy-Einstellungen gehören ausdrücklich nicht zur Sheet-Sync-GUI.

Das Servicekonto wird einmalig als JSON-Datei über eine Admin-Sitzung (HTTPS in
Produktion) geladen. Der Server validiert es vor der Übernahme und speichert
es in `sheet_sync_config.credentials_ciphertext` mit AES-256-GCM und zufälligem
Nonce. Ein separat erzeugter, nicht in der Datenbank abgelegter Schlüssel
liegt mit Modus 0600 in `<DB-Datei>.sheet-sync.key`; die API gibt ausschließlich
`credentials_configured` zurück. Schlüsseldatei und Datenbank müssen beim
Umzug gemeinsam gesichert und wiederhergestellt werden. Die Schlüsseldatei
gehört nicht in git und soll nur für den Serverprozess lesbar sein. Die
Verschlüsselung schützt gegen isolierte DB-Abzüge, nicht gegen die Übernahme
des gesamten Servers einschließlich Schlüsseldatei.

Der originale Sheet ist weiterhin niemals Syncquelle oder -ziel.
Als Quelle/Ziel wird weiterhin ausschließlich die explizit autorisierte
Kopie `10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8` akzeptiert.
Konfigurationseinträge ändern weder Raid-/Loot-/EP-Bestände noch
bestehende Import-Provenienz. Die Vorabprüfung bleibt erforderlich;
ein automatischer Lauf wird nur nach explizitem, eigenständigem Opt-in
aktiviert und bei Deaktivierung abgebrochen. Manuelle und geplante
Läufe sowie Konfigurationsänderungen sind serialisiert.
Eine ungültige/fehlende gespeicherte Zugangskonfiguration blockiert
nur Sync, nicht die gesamte Anmeldung, damit Admins sie korrigieren können.

## Konsequenzen

Migration 060 ergänzt allein die Einstellungstabelle. Ein neuer Server ohne
aktivierte Env-Vorgabe startet ohne Sync; Admins richten alles über die GUI ein.
Beim Upgrade mit existierenden Env-Werten bleibt das bisherige Verhalten
bestehen, bis die ersten GUI-Einstellungen gespeichert werden. Schlüsseldatei
und Datenbank sind für Backups gemeinsam notwendig. Live-Google-Tests
müssen ausdrücklich vor dem produktiven Autorun erfolgen.
