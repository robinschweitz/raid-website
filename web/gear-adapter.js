// Inserted into the private planner's startGearPlanner(setup) scope.
// V17 supplies render(), SPECS, SPEC_CLASS, byId(), and its local store.
// Intercept the renderer's wheel handlers before they reach its replaceable
// canvas. Leave the browser default alone so scrolling continues on the page.
for (const type of ['wheel', 'mousewheel', 'DOMMouseScroll']) {
  byId('model_3d').addEventListener(type, event => event.stopPropagation(), {capture:true, passive:true})
}
// Measure the content, not the viewport: a short filtered list must also shrink.
const frame = window.frameElement
if (frame) {
  const fit = () => { frame.style.height = `${Math.ceil(document.body.getBoundingClientRect().height)}px` }
  const observer = new ResizeObserver(fit)
  observer.observe(document.body)
  fit()
}
const classKey = {Todesritter:'DK', Krieger:'Warrior', Druide:'Druid', Schurke:'Rogue', Jäger:'Hunter', Hexer:'Warlock', Hexenmeister:'Warlock', Priester:'Priest', Schamane:'Shaman', Paladin:'Paladin', Magier:'Mage'}[setup.character.class]
const options = SPECS.filter(s => SPEC_CLASS[s.name] === classKey)
const raceID = {Mensch:1, Orc:2, Zwerg:3, Nachtelf:4, Untoter:5, Taure:6, Gnom:7, Troll:8, Blutelf:10, Draenei:11}[setup.profileRace]
const selectSpec = byId('spec')
selectSpec.replaceChildren(...options.map(s => new Option(s.name, s.name)))
if (!options.length) {
  document.body.textContent = 'Für diese Klasse ist keine BiS-Liste verfügbar.'
} else {
  activeChar = setup.character.name
  if (raceID) {
    byId('charRace').disabled = true
    byId('charRace').title = 'Volk aus dem verknüpften Rising-Gods-Profil'
  }
  const specHint = setup.character.main_spec.toLocaleLowerCase('de')
  activeSpec = options.find(s => s.name === store._last?.spec)?.name
    ?? options.find(s => specHint && s.name.toLocaleLowerCase('de').includes(specHint))?.name ?? options[0].name
  selectSpec.value = activeSpec
  refreshCharacters(); render()
}

function wishControls(name, candidates, source, mapped, owned) {
  const controls = document.createElement('span')
  if (!setup.addWish) return controls
  const variants = new Map()
  const add = (id, mode) => { if (/^[1-9]\d*$/.test(String(id))) variants.set(String(id), mode) }
  for (const candidate of candidates) add(candidate.id, candidate.mode)
  if (source) { add(source.normalId, 'Normal'); add(source.heroicId, 'Heroisch') }
  if (!variants.size && mapped) add(mapped.id, mapped.variant)
  if (!variants.size) return controls
  for (const [id, mode] of variants) {
    const remove = wished.has(id)
    if (owned && !remove) continue
    const button = document.createElement('button')
    button.type = 'button'
    const suffix = mode ? ' · ' + mode : ''
    button.disabled = saving.has(id)
    button.textContent = (remove ? 'Von Wunschliste entfernen' : 'Zur Wunschliste') + suffix
    button.onclick = async event => {
      event.preventDefault(); event.stopPropagation()
      if (saving.has(id) || wished.has(id) !== remove) return
      saving.add(id); button.disabled = true
      try {
        if (remove) { await setup.removeWish(id); wished.delete(id) }
        else { await setup.addWish(id, name); wished.add(id) }
        byId('wish-status').textContent = name + suffix + (remove ? ' wurde von der Wunschliste entfernt.' : ' steht auf der Wunschliste von ' + setup.character.name + '.')
      }
      catch { byId('wish-status').textContent = remove ? 'Wunsch konnte nicht entfernt werden. Bitte erneut versuchen.' : 'Wunsch konnte nicht gespeichert werden. Bitte erneut versuchen.' }
      finally { saving.delete(id); render() }
    }
    controls.append(button)
  }
  return controls
}
