function updateBossItemValidation_(e) {
  if (!e || !e.range) return;

  const MAIN_SHEET_NAME = "ICC25 Loot";
  const HELPER_SHEET_NAME = "Helpertabelle";

  const BOSS_COL = 1; // A
  const ITEM_COL = 2; // B

  const HELPER_START_ROW = 2;
  const HELPER_BOSS_COL = 4; // D
  const HELPER_ITEM_COL = 5; // E

  const sheet = e.range.getSheet();
  if (sheet.getName() !== MAIN_SHEET_NAME) return;

  const editedCol = e.range.getColumn();
  if (editedCol !== BOSS_COL) return;

  const row = e.range.getRow();
  if (row < 2) return; // skip header

  const ss = e.source;
  const helper = ss.getSheetByName(HELPER_SHEET_NAME);
  if (!helper) return;

  const boss = String(sheet.getRange(row, BOSS_COL).getValue()).trim();
  const itemCell = sheet.getRange(row, ITEM_COL);

  if (!boss) {
    itemCell.clearDataValidations();
    itemCell.clearContent();
    return;
  }

  const lastRow = helper.getLastRow();
  if (lastRow < HELPER_START_ROW) return;

  const data = helper
    .getRange(
      HELPER_START_ROW,
      HELPER_BOSS_COL,
      lastRow - HELPER_START_ROW + 1,
      2
    )
    .getValues();

  const items = [];
  for (const [b, it] of data) {
    const bb = String(b).trim();
    const ii = String(it).trim();
    if (!bb || !ii) continue;
    if (bb === boss) items.push(ii);
  }

  const uniqueItems = [...new Set(items)];

  if (uniqueItems.length === 0) {
    itemCell.clearDataValidations();
    return;
  }

  const rule = SpreadsheetApp.newDataValidation()
    .requireValueInList(uniqueItems, true)
    .setAllowInvalid(false)
    .build();

  itemCell.setDataValidation(rule);

  const current = String(itemCell.getValue()).trim();
  if (current && !uniqueItems.includes(current)) {
    itemCell.clearContent();
  }
}