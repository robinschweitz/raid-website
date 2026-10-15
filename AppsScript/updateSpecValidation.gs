function updateSpecValidation_(e) {
  const sh = e.range.getSheet();

  const row = e.range.getRow();
  const col = e.range.getColumn();

  const NAME_COL = 2; // B = Charakter
  const SPEC_COL = 5; // E = Spec
  if (row < 2 || col !== NAME_COL) return;

  SpreadsheetApp.flush(); // Formeln neu berechnen lassen

  const classCell = sh.getRange(row, 4); // D = Klasse (Formel)
  const chosenClass = String(classCell.getDisplayValue() || "").trim();
  const specCell = sh.getRange(row, SPEC_COL);

  // Wenn keine Klasse: Spec leeren
  if (!chosenClass) {
    specCell.clearContent();
    specCell.clearDataValidations();
    return;
  }

  const ss = e.source;
  const mapSh = ss.getSheetByName("Helpertabelle");
  if (!mapSh) return;

  const mapLast = mapSh.getLastRow();
  if (mapLast < 2) return;

  const mapData = mapSh.getRange(2, 1, mapLast - 1, 2).getDisplayValues(); // A:B

  const specs = mapData
    .filter(([cls, spec]) => String(cls).trim().toLowerCase() === chosenClass.toLowerCase() && spec)
    .map(([, spec]) => String(spec).trim());

  if (!specs.length) {
    specCell.clearDataValidations();
    return;
  }

  const unique = [...new Set(specs)].sort();
  const rule = SpreadsheetApp.newDataValidation()
    .requireValueInList(unique, true)
    .setAllowInvalid(false)
    .build();

  specCell.setDataValidation(rule);

  // Spec leeren, falls jetzt ungültig
  const current = String(specCell.getDisplayValue() || "").trim();
  if (current && !unique.includes(current)) specCell.clearContent();
}
