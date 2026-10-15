/**
 * ICC25 Raidsetup – Gruppeneinteilung (Google Apps Script)
 *
 * Passt zu deiner Tabelle:
 * Slot | Charakter | Spieler | Klasse | Spec | Rolle | Draenei (Ja/Nein) | Manuelle Gruppe (1-5)
 *
 * Output:
 * - Schreibt Ergebnis in Spalte "Finale Gruppe" (wird angelegt falls fehlt)
 * - Optional: erstellt/updated "Raid Overview"
 *
 * Regeln:
 * - Tanks immer Gruppe 1
 * - Heiler immer Gruppe 5
 * - Melee (Mdd) -> Gruppen 1,2,3,4 (wenn voll: nächstes)
 * - Ranged (Rdd) -> Gruppen 3,4,2,1 (5 fällt weg, weil Heiler fix in 5)
 * - Draenei: bleiben in ihrer Role-Logik, aber werden verteilt wenn möglich (1 Draenei pro Gruppe, soweit möglich)
 * - Manuelle Gruppe (1-5): überschreibt alles
 *
 * WICHTIG:
 * - DPS werden in 2 Phasen verarbeitet: zuerst MELEE, dann RANGED
 *   (damit Ranged nicht die "guten" Plätze für Melee wegnimmt).
 */

const CFG = {
  rosterSheetName: "ICC25 Raidsetup",   // <-- Tabname exakt!
  overviewSheetName: "Raid Overview",   // optional

  headers: {
    slot: "Slot",
    charakter: "Charakter",
    rolle: "Rolle",
    draenei: "Draenei (Ja/Nein)",
    manual: "Manuelle Gruppe (1-5)",
    output: "Finale Gruppe",
    position: "Position"
  },

  capacityPerGroup: 5,                 // 25er Raid -> 5 pro Gruppe

  // Wenn Tanks/Heiler die Gruppe überfüllen würden:
  // true = trotzdem rein (Regel "immer"), wird in Overview als Overflow markiert
  allowOverflowForLockedRoles: true
};

/** Hauptfunktion: liest Tabelle, berechnet Gruppen, schreibt Output + Overview */
function buildRaidGroupsExact() {
  const ss = SpreadsheetApp.getActive();
  const sh = ss.getSheetByName(CFG.rosterSheetName);
  if (!sh) throw new Error(`Sheet nicht gefunden: "${CFG.rosterSheetName}"`);

  const range = sh.getDataRange();
  const values = range.getValues();
  if (values.length < 2) return;

  const headers = values[0].map(h => String(h).trim());
  const idx = indexMap_(headers);

  // Required headers prüfen
  const req = [CFG.headers.charakter, CFG.headers.rolle, CFG.headers.manual];
  req.forEach(h => {
    if (idx[h] == null) throw new Error(`Header fehlt in Zeile 1: "${h}"`);
  });

  // Output-Spalte anlegen falls nötig
  const outCol = 10 - 1; // Spalte J (0-based Index)
  sh.getRange(1, 10).setValue(CFG.headers.output);

  const capacity = {1: CFG.capacityPerGroup, 2: CFG.capacityPerGroup, 3: CFG.capacityPerGroup, 4: CFG.capacityPerGroup, 5: CFG.capacityPerGroup};

  // Spieler sammeln: jede Zeile mit Charakter != leer
  const players = [];
  for (let r = 1; r < values.length; r++) {
    const row = values[r];
    const charakter = String(row[idx[CFG.headers.charakter]] || "").trim();
    if (!charakter) continue;

    const rolleRaw = String(row[idx[CFG.headers.rolle]] || "").trim();
    const roleKey = normalizeRoleKey_(rolleRaw);
    if (!roleKey) continue;

    const draeneiRaw = idx[CFG.headers.draenei] != null ? String(row[idx[CFG.headers.draenei]] || "").trim() : "";
    const isDraenei = /^ja$/i.test(draeneiRaw);

    const manualRaw = String(row[idx[CFG.headers.manual]] || "").trim();
    const manualGroup = parseManualGroup_(manualRaw); // null oder 1..5

    players.push({
      rowIndex: r + 1,     // sheet row (1-based)
      charakter,
      rolleRaw,
      roleKey,             // "tank"|"heal"|"melee"|"ranged"
      isDraenei,
      manualGroup
    });
  }

  const result = assignGroups_(players, capacity);

  // Output schreiben
  const lastRow = values.length;
  const outRange = sh.getRange(2, outCol + 1, lastRow - 1, 1);
  const outVals = outRange.getValues();

  const byRow = result.assignmentByRow;
  const processed = new Set(players.map(p => p.rowIndex));

  for (let sheetRow = 2; sheetRow <= lastRow; sheetRow++) {
    const a = sheetRow - 2;
    if (!processed.has(sheetRow)) continue;

    const g = byRow[sheetRow];
    outVals[a][0] = g != null ? g : "UNASSIGNED";
  }

  // outRange.setValues(outVals);

  // Overview sheet schreiben
  // writeOverview_(ss, result);
  writePositions_(sh, result);

}

/* ========================= Assignment Logic ========================= */

function assignGroups_(players, capacityByGroup) {
  const groupMembers = {1:[],2:[],3:[],4:[],5:[]};
  const draeneiCount = {1:0,2:0,3:0,4:0,5:0};
  const assignmentByRow = {};
  const overflow = [];
  const unassigned = [];

  // Deine Regeln als Präferenzen:
  const PREF = {
    tank:   [1],
    heal:   [5],
    melee:  [1, 2, 3, 4],
    ranged: [3, 4, 5, 2, 1]
  };

  function isFull(g) { return groupMembers[g].length >= capacityByGroup[g]; }
  function hasDraenei(g) { return draeneiCount[g] > 0; }

  function place(p, g, isOverflow=false) {
    groupMembers[g].push(p);
    assignmentByRow[p.rowIndex] = g;
    if (p.isDraenei) draeneiCount[g]++;
    if (isOverflow) overflow.push({p, g});
  }

  // 0) Manuelle Gruppen zuerst (Override)
  const manual = players.filter(p => p.manualGroup != null);
  manual.forEach(p => {
    const g = p.manualGroup;
    const isOver = groupMembers[g].length >= capacityByGroup[g];
    place(p, g, isOver);
  });

  const rest = players.filter(p => p.manualGroup == null);

  // 1) Tanks -> G1 (hart)
  rest.filter(p => p.roleKey === "tank").forEach(p => {
    if (!isFull(1)) place(p, 1);
    else if (CFG.allowOverflowForLockedRoles) place(p, 1, true);
    else unassigned.push(p);
  });

  // 2) Heiler -> G5 (hart)
  rest.filter(p => p.roleKey === "heal").forEach(p => {
    if (!isFull(5)) place(p, 5);
    else if (CFG.allowOverflowForLockedRoles) place(p, 5, true);
    else unassigned.push(p);
  });

  // 3) DPS in 2 Phasen: erst MELEE, dann RANGED
  function assignDpsPhase(roleKey) {
    const list = rest
      .filter(p => p.roleKey === roleKey)
      .filter(p => assignmentByRow[p.rowIndex] == null);

    // Draenei zuerst (Verteilung), dann stabil nach Name
    list.sort((a, b) => {
      if (a.isDraenei !== b.isDraenei) return a.isDraenei ? -1 : 1;
      return a.charakter.localeCompare(b.charakter);
    });

    list.forEach(p => {
      const prefs = PREF[p.roleKey];

      // 1) HARD: Draenei zuerst in Gruppen ohne Draenei (innerhalb erlaubter prefs)
      if (p.isDraenei) {
        for (let i = 0; i < prefs.length; i++) {
          const g = prefs[i];
          if (!isFull(g) && !hasDraenei(g)) {
            place(p, g);
            return;
          }
        }
      }

      // 2) Fallback: normale Präferenz-Reihenfolge (mit leichtem Balancing)
      for (let i = 0; i < prefs.length; i++) {
        const g = prefs[i];
        if (!isFull(g)) {
          place(p, g);
          return;
        }
      }

      // 3) Wenn wirklich nix geht
      unassigned.push(p);
    });
  }

  assignDpsPhase("melee");
  assignDpsPhase("ranged");

  return { groupMembers, draeneiCount, assignmentByRow, overflow, unassigned };
}

/* ========================= Helpers ========================= */

function indexMap_(headers) {
  const map = {};
  headers.forEach((h, i) => { if (h) map[h] = i; });
  return map;
}

function parseManualGroup_(s) {
  if (!s) return null;
  const n = Number(String(s).trim());
  if (Number.isInteger(n) && n >= 1 && n <= 5) return n;
  return null;
}

// Wandelt deine Rollenwerte in Keys um:
// Tank -> "tank"
// Heiler -> "heal"
// Mdd -> "melee"
// Rdd -> "ranged"
function normalizeRoleKey_(roleRaw) {
  const s = String(roleRaw || "").trim().toLowerCase();
  if (!s) return null;

  if (s === "tank") return "tank";
  if (s === "heiler") return "heal";

  if (s === "mdd") return "melee";
  if (s === "rdd") return "ranged";

  // Falls du andere Schreibweisen hast, hier ergänzen:
  return null;
}

function writePositions_(sh, result) {
  // Position soll in Spalte I stehen (1-based Column = 9)
  const POS_COL = 9;

  // Header setzen
  sh.getRange(1, POS_COL).setValue(CFG.headers.position);

  // Alte Positionen löschen (nur Datenbereich)
  const lastRow = sh.getLastRow();
  if (lastRow >= 2) sh.getRange(2, POS_COL, lastRow - 1, 1).clearContent();

  const posByRow = {}; // rowIndex -> position

  // Positionen anhand der Gruppenlisten vergeben (1..25)
  for (let g = 1; g <= 5; g++) {
    const members = result.groupMembers[g] || [];
    for (let i = 0; i < members.length; i++) {
      // innerhalb der Gruppe Platz 1..5
      const pos = (g - 1) * CFG.capacityPerGroup + (i + 1);
      posByRow[members[i].rowIndex] = pos;
    }
  }

  // Positionswerte in einem Rutsch schreiben
  const values = [];
  for (let r = 2; r <= lastRow; r++) {
    values.push([posByRow[r] || ""]);
  }
  sh.getRange(2, POS_COL, values.length, 1).setValues(values);
}
