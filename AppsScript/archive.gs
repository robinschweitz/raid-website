/************************************
 * ARCHIVE RAID + LOOT + BONUS
 ************************************/

const BONUS_CFG = {
  pointsSheetName: "Punkte",
  bonusAmount: 10,
  enhanceLabel: "Enhanced",
  mainLabel: "Main" 
};

/************************************
 * DISABLE LOOT POINTS (for clearing)
 ************************************/
function setLootDisabled_(value) {
  PropertiesService.getDocumentProperties().setProperty("LOOT_DISABLED", value ? "1" : "0");
}
function isLootDisabled_() {
  return PropertiesService.getDocumentProperties().getProperty("LOOT_DISABLED") === "1";
}

function parseGermanDate_(dateStr) {
  const match = /^(\d{2})\.(\d{2})\.(\d{4})$/.exec(dateStr);
  if (!match) {
    throw new Error(`Invalid date format: "${dateStr}". Use DD.MM.YYYY (e.g. 23.02.2026).`);
  }

  const day = Number(match[1]);
  const month = Number(match[2]); // 1–12
  const year = Number(match[3]);

  // Use noon to avoid timezone/DST edge cases
  return new Date(year, month - 1, day, 12, 0, 0);
}

function archiveRaidWithLoot() {
  const ss = SpreadsheetApp.getActive();
  const ui = SpreadsheetApp.getUi();

  const rosterSheet = ss.getSheetByName("ICC25 Raidsetup");
  if (!rosterSheet) throw new Error("Sheet 'ICC25 Raidsetup' not found");

  const lootSheet = ss.getSheetByName("ICC25 Loot");
  if (!lootSheet) throw new Error("Sheet 'ICC25 Loot' not found");

  // Ask for Raid ID
  const res = ui.prompt("Archive Raid", "Enter Raid ID (e.g. ICC25-11)", ui.ButtonSet.OK_CANCEL);
  if (res.getSelectedButton() !== ui.Button.OK) return;
  const raidId = res.getResponseText().trim();
  if (!raidId) return ui.alert("Raid ID cannot be empty.");

  // Ask for raid date (format: DD.MM.YYYY)
  const dateRes = ui.prompt(
    "Archive Raid",
    "Enter Raid Date (DD.MM.YYYY), e.g. 23.02.2026.\nLeave empty to use today.",
    ui.ButtonSet.OK_CANCEL
  );

  if (dateRes.getSelectedButton() !== ui.Button.OK) return;

  const dateStr = dateRes.getResponseText().trim();
  const timestamp = dateStr ? parseGermanDate_(dateStr) : new Date();

  // =========================
  // RAID ARCHIVE (Positions 1-25)
  // =========================
  let raidArchive = ss.getSheetByName("Raid Archive");
  if (!raidArchive) {
    raidArchive = ss.insertSheet("Raid Archive");
    raidArchive.appendRow(["Raid ID", "Timestamp", "Position", "Charakter", "Rolle", "Klasse", "Spec"]);
  }

  const rosterData = rosterSheet.getDataRange().getValues();
  const rosterHeaders = rosterData[0].map(h => String(h).trim());

  const rCharIdx = rosterHeaders.indexOf("Charakter");
  const rRoleIdx = rosterHeaders.indexOf("Rolle");
  const rPosIdx  = rosterHeaders.indexOf("Position");
  const rClassIdx  = rosterHeaders.indexOf("Klasse");
  const rSpecIdx  = rosterHeaders.indexOf("Spec");

  if (rCharIdx === -1 || rPosIdx === -1) {
    throw new Error("Roster sheet needs columns: 'Charakter' and 'Position'");
  }

  const rosterRows = [];
  for (let i = 1; i < rosterData.length; i++) {
    const row = rosterData[i];
    const pos = Number(row[rPosIdx]);
    if (pos >= 1 && pos <= 25) {
      rosterRows.push([
        raidId,
        timestamp,
        pos,
        row[rCharIdx],
        rRoleIdx !== -1 ? row[rRoleIdx] : "",
        rClassIdx !== -1 ? row[rClassIdx] : "",
        rSpecIdx !== -1 ? row[rSpecIdx] : ""
      ]);
    }
  }

  if (rosterRows.length === 0) {
    ui.alert("No active raid roster found (no Positions 1–25).");
    return;
  }

  rosterRows.sort((a, b) => Number(a[2]) - Number(b[2]));

  raidArchive.getRange(
    raidArchive.getLastRow() + 1,
    1,
    rosterRows.length,
    rosterRows[0].length
  ).setValues(rosterRows);

  // =========================
  // LOOT ARCHIVE (from ICC25 Loot)
  // Columns archived: Raid ID, Timestamp, Character, Loot, Priority
  // =========================
  let lootArchive = ss.getSheetByName("Loot Archive");
  if (!lootArchive) {
    lootArchive = ss.insertSheet("Loot Archive");
    lootArchive.appendRow(["Raid ID", "Timestamp", "Character", "Loot", "Priority"]);
  }

  const lootData = lootSheet.getDataRange().getValues();
  let lootRows = [];

  if (lootData.length >= 2) {
    const lootHeaders = lootData[0].map(h => String(h).trim());
    const bossIdx = lootHeaders.indexOf("Boss");
    const itemIdx = lootHeaders.indexOf("Item");
    const anIdx   = lootHeaders.indexOf("An");
    const prioIdx = lootHeaders.indexOf("Vergabe");

    if (itemIdx === -1 || anIdx === -1 || prioIdx === -1) {
      throw new Error("Loot sheet needs columns: 'Item', 'An', 'Vergabe' (Boss optional)");
    }

    for (let i = 1; i < lootData.length; i++) {
      const row = lootData[i];
      const character = String(row[anIdx] || "").trim();
      const item = String(row[itemIdx] || "").trim();
      const prio = String(row[prioIdx] || "").trim();

      if (!character || !item) continue;

      // Optional boss in loot text:
      // const boss = bossIdx !== -1 ? String(row[bossIdx] || "").trim() : "";
      // const lootText = boss ? `${item} (${boss})` : item;

      lootRows.push([raidId, timestamp, character, item, prio]);
    }

    if (lootRows.length > 0) {
      lootArchive.getRange(
        lootArchive.getLastRow() + 1,
        1,
        lootRows.length,
        lootRows[0].length
      ).setValues(lootRows);
    }
  } else {
    ui.alert("Raid archived, but no loot rows found in 'ICC25 Loot'.");
  }

  // =========================
  // BONUS: +10 for players in raid roster (pos 1-25) who got NO Main loot
  // =========================
  let bonusReport = null;
  try {
    bonusReport = applyNoMainLootBonus_(ss, rosterSheet, lootSheet, raidId);
  } catch (err) {
    console.error("applyNoMainLootBonus_ error:", err);
    ui.alert("Archive completed, but bonus points failed. Check Apps Script logs.");
  }

  // =========================
  // CLEAR LOOT SHEET (NO REFUNDS)
  // =========================
  try {
    setLootDisabled_(true);

    const lastRow = lootSheet.getLastRow();
    const lastCol = lootSheet.getLastColumn();

    if (lastRow > 1 && lastCol > 0) {
      const numRows = lastRow - 1; // from row 2

      // Clear columns B..D (2..4)
      if (lastCol >= 2) {
        const endColBD = Math.min(4, lastCol);
        if (endColBD >= 2) {
          lootSheet.getRange(2, 2, numRows, endColBD - 2 + 1).clearContent();
        }
      }

      // Skip E (5)

      // Clear columns F..lastCol (6..lastCol)
      if (lastCol >= 6) {
        lootSheet.getRange(2, 6, numRows, lastCol - 6 + 1).clearContent();
      }
    }

    console.log("Loot sheet cleared after archive (loot points disabled).");
  } finally {
    setLootDisabled_(false);
  }

  // Build final overview message
  const bonusText = bonusReport ? formatBonusReport_(bonusReport) : "Bonus report: (failed)";

  ui.alert(
    `Archived!\n` +
    `Roster entries: ${rosterRows.length}\n` +
    `Loot entries: ${lootRows.length}\n` +
    `Raid ID: ${raidId}\n\n` +
    bonusText
  );

  return; // prevent the old ui.alert below if you still have it
}

function applyNoMainLootBonus_(ss, rosterSheet, lootSheet, raidId) {
  const pointsSheet = ss.getSheetByName(BONUS_CFG.pointsSheetName);
  if (!pointsSheet) throw new Error(`Sheet "${BONUS_CFG.pointsSheetName}" not found`);

  // --- Read roster (active positions 1..25) ---
  const rosterData = rosterSheet.getDataRange().getValues();
  const rosterHeaders = rosterData[0].map(h => String(h).trim());

  const rCharIdx   = rosterHeaders.indexOf("Charakter");
  const rPlayerIdx = rosterHeaders.indexOf("Spieler");
  const rPosIdx    = rosterHeaders.indexOf("Position");

  if (rCharIdx === -1 || rPlayerIdx === -1 || rPosIdx === -1) {
    throw new Error(`Raidsetup needs columns: "Charakter", "Spieler", "Position"`);
  }

  const charToPlayer = {};
  const raidPlayersSet = new Set();
  const missingRosterPlayers = []; // chars in raid with blank Spieler

  for (let i = 1; i < rosterData.length; i++) {
    const row = rosterData[i];
    const pos = Number(row[rPosIdx]);
    if (!(pos >= 1 && pos <= 25)) continue;

    const ch = String(row[rCharIdx] || "").trim();
    const pl = String(row[rPlayerIdx] || "").trim();

    if (ch) charToPlayer[ch.toLowerCase()] = pl; // may be ""
    if (pl) raidPlayersSet.add(pl.toLowerCase());
    if (ch && !pl) missingRosterPlayers.push(ch);
  }

  const raidPlayers = Array.from(raidPlayersSet);

  // --- Determine which PLAYERS got at least one MAIN loot ---
  const lootData = lootSheet.getDataRange().getValues();
  const mainLootPlayers = new Set();
  const unmappedMainLootCharacters = []; // got main loot but no mapped player

  if (lootData.length >= 2) {
    const lootHeaders = lootData[0].map(h => String(h).trim());
    const anIdx   = lootHeaders.indexOf("An");
    const itemIdx = lootHeaders.indexOf("Item");
    const prioIdx = lootHeaders.indexOf("Vergabe");

    if (anIdx === -1 || itemIdx === -1 || prioIdx === -1) {
      throw new Error(`ICC25 Loot needs columns: "An", "Item", "Vergabe"`);
    }

    for (let i = 1; i < lootData.length; i++) {
      const row = lootData[i];
      const character = String(row[anIdx] || "").trim();
      const item = String(row[itemIdx] || "").trim();
      const prio = String(row[prioIdx] || "").trim();

      if (!character || !item) continue;
      const validLabels = [
        BONUS_CFG.mainLabel,
        BONUS_CFG.enhanceLabel
      ].map(label => label.toLowerCase());

      if (!validLabels.includes(prio.toLowerCase())) continue;

      const mappedPlayer = (charToPlayer[character.toLowerCase()] || "").trim();
      if (mappedPlayer) {
        mainLootPlayers.add(mappedPlayer.toLowerCase());
      } else {
        unmappedMainLootCharacters.push(character);
      }
    }
  }

  // --- Apply bonus to raid players who did NOT get MAIN ---
  const bonusApplied = [];      // {player, delta}
  const noBonusMain = [];       // players that got main
  const missingInPoints = [];   // players in raid but not in Points sheet

  // Pre-check: Points name set for fast membership test
  const pointsValues = pointsSheet.getDataRange().getValues();
  const pointsNameSet = new Set(
    pointsValues.slice(1).map(r => String(r[0] || "").trim().toLowerCase()).filter(Boolean)
  );

  for (const pLower of raidPlayers) {
    const playerName = findOriginalCasing_(rosterData, rosterHeaders, "Spieler", pLower) || pLower;

    if (mainLootPlayers.has(pLower)) {
      noBonusMain.push(playerName);
      continue;
    }

    if (!pointsNameSet.has(pLower)) {
      missingInPoints.push(playerName);
      continue;Version
    }

    // Apply +10 (and log “Player X got added 10 points”)
    updatePlayerPointsNoClampLog_(pointsSheet, playerName, BONUS_CFG.bonusAmount);
    bonusApplied.push({ player: playerName, delta: BONUS_CFG.bonusAmount });
  }

  console.log(`BONUS: Raid ${raidId} -> +${BONUS_CFG.bonusAmount} to ${bonusApplied.length} players (no Main loot)`);

  return {
    raidId,
    bonusAmount: BONUS_CFG.bonusAmount,
    bonusApplied,
    noBonusMain,
    missingInPoints,
    missingRosterPlayers,         // characters in roster with blank Spieler
    unmappedMainLootCharacters    // characters with main loot but no mapped Spieler
  };
}

function updatePlayerPointsNoClampLog_(pointsSheet, playerName, delta) {
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
  const next = current + delta;

  cell.setValue(next);

  if (delta !== 0) {
    if (delta > 0) {
      console.log(`Player "${playerName}" got added ${delta} points`);
    } else {
      console.log(`Player "${playerName}" got removed ${Math.abs(delta)} points`);
    }
  }

  return delta;
}

function findOriginalCasing_(rosterData, rosterHeaders, headerName, lowerValue) {
  const idx = rosterHeaders.indexOf(headerName);
  if (idx === -1) return "";

  for (let i = 1; i < rosterData.length; i++) {
    const v = String(rosterData[i][idx] || "").trim();
    if (v && v.toLowerCase() === lowerValue) return v;
  }
  return "";
}

function formatBonusReport_(report) {
  const lines = [];

  // Bonus applied
  if (report.bonusApplied.length) {
    lines.push(`+${report.bonusAmount} applied to (${report.bonusApplied.length}):`);
    lines.push(report.bonusApplied.map(x => `  - ${x.player}: +${x.delta}`).join("\n"));
  } else {
    lines.push(`+${report.bonusAmount} applied to (0): none`);
  }

  // No bonus because main loot
  if (report.noBonusMain.length) {
    lines.push(`\nNo bonus (got Main loot) (${report.noBonusMain.length}):`);
    lines.push(report.noBonusMain.map(p => `  - ${p}: +0`).join("\n"));
  }

  // Missing in Points sheet
  if (report.missingInPoints.length) {
    lines.push(`\nMissing in Points sheet (${report.missingInPoints.length}):`);
    lines.push(report.missingInPoints.map(p => `  - ${p}`).join("\n"));
  }

  // Missing Spieler mapping in roster
  if (report.missingRosterPlayers.length) {
    lines.push(`\nRoster characters with empty Spieler (${report.missingRosterPlayers.length}):`);
    lines.push(report.missingRosterPlayers.map(c => `  - ${c}`).join("\n"));
  }

  // Main loot characters with no mapped Spieler
  if (report.unmappedMainLootCharacters.length) {
    lines.push(`\nMain loot to characters with no mapped Spieler (${report.unmappedMainLootCharacters.length}):`);
    lines.push(report.unmappedMainLootCharacters.map(c => `  - ${c}`).join("\n"));
  }

  return lines.join("\n");
}