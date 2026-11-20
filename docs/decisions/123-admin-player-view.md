# 123 – Spieleransicht für den Support

Entschieden am 2026-11-19 (#247). Ergänzt ADR 010 und 121. Der Support verwendet eine an die bestehende MFA-Admin-Sitzung gebundene Ansicht, keine Sitzung oder Zugangsdaten des Zielkontos. Sie startet lesend. Nur ausdrücklich freigegebene Spieleraktionen dürfen im Schreibmodus die bestehenden Fachabläufe aufrufen. Die Auditierung ordnet Änderungen dem tatsächlichen Admin und dem vertretenen Spieler zu (FR-2.15).
