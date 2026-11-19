# BiS-Gear-Manager einrichten

Einmal pro V17-/Viewer-Version als Betreiber mit Node und den installierten `web`-Abhängigkeiten ausführen. Das entpackte, zur Nutzung freigegebene Komplettpaket muss außerhalb des Repositorys und aller öffentlichen Webverzeichnisse liegen:

```sh
cd web
npm ci
node prepare-gear.mjs /srv/raid-gear/V17
```

Der Pfad enthält `V17_Teststand`, `meta`, `bone`, `mo3`, `textures` und `icons`. Das Skript prüft den V17-Quelltext, baut den fest versionierten Viewer und legt die angepasste Anwendung einschließlich prüfsummenvalidiertem jQuery unter `private-ui` daneben. Alternativ auf dem Build-Rechner vorbereiten und die vorbereiteten Verzeichnisse übertragen. Die Spieler benötigen keinen Download und keinen lokalen Server.

`GEAR_MANAGER_DIR=/srv/raid-gear/V17` in der Dienstumgebung setzen, Dienst neu starten. Der Dienst benötigt Leserechte, keine Schreibrechte. Keine nginx-Static-Route anlegen. Ohne Konfiguration antwortet der geschützte Einstieg mit 503. Fortschritt ist browser- und kontogebunden; ein V17-Backup kann direkt im Manager importiert werden.

Nach einem Update der Integration `prepare-gear.mjs` erneut ausführen. Prüfung: Ohne Sitzung müssen `/api/v1/gear-manager/` und einzelne Dateien mit 401 antworten. Nach dem Login unter `/#/gear-manager` einen eigenen Profilcharakter wählen, ein fehlendes Item auf dessen Wunschliste setzen und einen Helm anprobieren. Der optionale Playwright-Test `e2e/gear-manager.spec.ts` nutzt das vorbereitete Verzeichnis über dieselbe Umgebungsvariable.
