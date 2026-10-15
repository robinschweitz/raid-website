function onEdit(e) {
  if (!e || !e.range) return;

  const sh = e.range.getSheet();
  const sheetName = sh.getName();
  const editedRow = e.range.getRow();
  const editedCol = e.range.getColumn();

  // Ignore header
  if (editedRow === 1) return;

  // === 1) Raidsetup logic ===
  if (sheetName === "ICC25 Raidsetup") {
    const RELEVANT_COLUMNS = [2, 5, 6, 7, 8];

    if (editedCol === 2) {
      try {
        updateSpecValidation_(e);
      } catch (err) {
        console.error("updateSpecValidation_ error:", err);
      }
    }

    if (RELEVANT_COLUMNS.includes(editedCol)) {
      try {
        buildRaidGroupsExact();
      } catch (err) {
        console.error("Group rebuild error:", err);
      }
    }
  }

  try {
    updateBossItemValidation_(e);
  } catch (err) {
    console.error("Boss item validation error:", err);
  }

  try {
    handleLootPointsOnEdit_(e);
  } catch (err) {
    console.error("Loot points error:", err);
  }
}

function onOpen() {
  SpreadsheetApp.getUi()
    .createMenu("Raid Tools")
    .addItem("Archive Current Raid", "archiveRaidWithLoot")
    .addToUi();
}
