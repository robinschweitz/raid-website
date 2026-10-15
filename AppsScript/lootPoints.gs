/***********************
 * Loot Points Module
 ***********************/
const LOOT_POINTS_CFG = {
  lootSheetName: "ICC25 Loot",        // <-- your loot tab name
  pointsSheetName: "Punkte",          // <-- your points tab name

  // Loot sheet columns (1-based)
  colItem: 2, // Item
  colCharacter: 3,      // <-- column where Charakter is on the loot sheet
  colSpec: 4,           // Main/Second
  colReturned: 6,       // checkbox
  colAppliedDelta: 7,   // helper (hide)
  colAppliedTo: 8,      // helper (hide)  <- stores SPIELER, not character

  enhanceLabel: "Enhanced",
  mainCost: 50,

  // Mapping table in ICC25 Raidsetup
  rosterSheetName: "Charaktere",
  rosterCharCol: 1,     // A = Charakter
  rosterPlayerCol: 2,   // B = Spieler
  rosterHeaderRow: 1
};

function handleLootPointsOnEdit_(e) {
  if (!e || !e.range) return;

  const sh = e.range.getSheet();
  if (sh.getName() !== LOOT_POINTS_CFG.lootSheetName) return;

  const row = e.range.getRow();
  const col = e.range.getColumn();
  if (row < 2) return; // header

  // React when character/spec/returned changes
  const relevant = [
    LOOT_POINTS_CFG.colCharacter,
    LOOT_POINTS_CFG.colSpec,
    LOOT_POINTS_CFG.colReturned
  ];
  if (!relevant.includes(col)) return;

  const lock = LockService.getDocumentLock();
  if (!lock.tryLock(5000)) return;

  try {
    const ss = e.source;
    const loot = ss.getSheetByName(LOOT_POINTS_CFG.lootSheetName);
    const points = ss.getSheetByName(LOOT_POINTS_CFG.pointsSheetName);
    if (!points) throw new Error(`Points sheet "${LOOT_POINTS_CFG.pointsSheetName}" not found`);

    const maxCol = Math.max(
      LOOT_POINTS_CFG.colItem,
      LOOT_POINTS_CFG.colCharacter,
      LOOT_POINTS_CFG.colSpec,
      LOOT_POINTS_CFG.colReturned,
      LOOT_POINTS_CFG.colAppliedDelta,
      LOOT_POINTS_CFG.colAppliedTo
    );

    const rowValues = loot.getRange(row, 1, 1, maxCol).getValues()[0];

    const item = (rowValues[LOOT_POINTS_CFG.colItem - 1] || "").toString().trim();
    const character = (rowValues[LOOT_POINTS_CFG.colCharacter - 1] || "").toString().trim();
    const spec = (rowValues[LOOT_POINTS_CFG.colSpec - 1] || "").toString().trim();
    const returned = Boolean(rowValues[LOOT_POINTS_CFG.colReturned - 1]);

    const appliedDeltaCell = loot.getRange(row, LOOT_POINTS_CFG.colAppliedDelta);
    const appliedToCell = loot.getRange(row, LOOT_POINTS_CFG.colAppliedTo);

    const oldAppliedDelta = Number(appliedDeltaCell.getValue() || 0);
    const oldAppliedToPlayer = (appliedToCell.getValue() || "").toString().trim(); // SPIELER

    console.log(`ITEM: Handeling Item "${item}"`);

    // Resolve owner (Spieler) from Charakter
    const ownerPlayer = resolvePlayerFromCharacter_(ss, character);

    // 1) Reverse previous application (from old owner stored)
    if (oldAppliedDelta !== 0 && oldAppliedToPlayer) {
      updatePlayerPointsClamp0_(points, oldAppliedToPlayer, -oldAppliedDelta);
    }

    // 2) Determine new desired delta
    const isMain = spec.toLowerCase() === LOOT_POINTS_CFG.enhanceLabel.toLowerCase();
    let desiredDelta = 0;

    if (ownerPlayer && isMain && !returned) {
      desiredDelta = -LOOT_POINTS_CFG.mainCost;
    }

    // 3) Apply new delta to resolved owner
    let actualAppliedDelta = 0;
    if (ownerPlayer && desiredDelta !== 0) {
      actualAppliedDelta = updatePlayerPointsClamp0_(points, ownerPlayer, desiredDelta);
    }

    // 4) Store what happened
    appliedDeltaCell.setValue(actualAppliedDelta);
    appliedToCell.setValue(actualAppliedDelta !== 0 ? ownerPlayer : "");

  } finally {
    lock.releaseLock();
  }
}

/**
 * Update player's points by delta, clamped so it never goes below 0.
 * Returns the actual delta applied (may be smaller in magnitude if clamped).
 */
function updatePlayerPointsClamp0_(pointsSheet, playerName, delta) {
  const nameCol = 1;  // A
  const valueCol = 2; // B

  const values = pointsSheet.getDataRange().getValues();
  let foundRow = -1;

  for (let i = 0; i < values.length; i++) {
    const name = (values[i][nameCol - 1] || "").toString().trim();
    if (name && name.toLowerCase() === playerName.toLowerCase()) {
      foundRow = i + 1;
      break;
    }
  }

  if (foundRow === -1) {
    SpreadsheetApp.getActive().toast(`Player "${playerName}" not found in "${pointsSheet.getName()}"`);
    return 0;
  }

  const cell = pointsSheet.getRange(foundRow, valueCol);
  const current = Number(cell.getValue() || 0);

  let next = current + delta;
  let applied = delta;

  if (next < 0) {
    next = 0;
    applied = -current; // only subtract what they had
  }

  cell.setValue(next);

    // LOG: only if something actually changed
  if (applied !== 0) {
    if (applied > 0) {
      console.log(`POINTS: Player "${playerName}" got added ${applied} points (from ${current} -> ${next})`);
    } else {
      console.log(`POINTS: Player "${playerName}" got removed ${Math.abs(applied)} points (from ${current} -> ${next})`);
    }
  }
  
  return applied;
}

function resolvePlayerFromCharacter_(ss, character) {
  if (!character || !character.trim()) return "";

  const cache = CacheService.getDocumentCache();
  const cacheKey = "CHAR_TO_PLAYER_MAP_V1";
  let mapJson = cache.get(cacheKey);

  let map;
  if (mapJson) {
    map = JSON.parse(mapJson);
  } else {
    map = buildCharToPlayerMap_(ss);
    cache.put(cacheKey, JSON.stringify(map), 300); // 5 min
  }

  return map[character.trim().toLowerCase()] || "";
}

function buildCharToPlayerMap_(ss) {
  const roster = ss.getSheetByName(LOOT_POINTS_CFG.rosterSheetName);
  if (!roster) throw new Error(`Roster sheet "${LOOT_POINTS_CFG.rosterSheetName}" not found`);

  const startRow = (LOOT_POINTS_CFG.rosterHeaderRow || 1) + 1;
  const lastRow = roster.getLastRow();
  if (lastRow < startRow) return {};

  const numRows = lastRow - startRow + 1;

  const charCol = LOOT_POINTS_CFG.rosterCharCol;
  const playerCol = LOOT_POINTS_CFG.rosterPlayerCol;

  const chars = roster.getRange(startRow, charCol, numRows, 1).getValues();
  const players = roster.getRange(startRow, playerCol, numRows, 1).getValues();

  const map = {};
  for (let i = 0; i < numRows; i++) {
    const c = (chars[i][0] || "").toString().trim();
    const p = (players[i][0] || "").toString().trim();
    if (c) map[c.toLowerCase()] = p; // may be "" if Spieler blank
  }
  return map;
}