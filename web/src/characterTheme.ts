export function characterClassName(name: string): string {
  const classes: Record<string, string> = {
    Todesritter: 'deathknight', 'Death Knight': 'deathknight', Druide: 'druid', Druid: 'druid',
    Jäger: 'hunter', Hunter: 'hunter', Magier: 'mage', Mage: 'mage', Paladin: 'paladin',
    Priester: 'priest', Priest: 'priest', Schamane: 'shaman', Shaman: 'shaman',
    Schurke: 'rogue', Rogue: 'rogue', Hexer: 'warlock', Hexenmeister: 'warlock', Warlock: 'warlock',
    Krieger: 'warrior', Warrior: 'warrior',
  }
  return `class-${classes[name] ?? 'unknown'}`
}
