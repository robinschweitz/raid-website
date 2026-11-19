// Run once as operator. No private input belongs in the build or web/public.
import { readFile, writeFile, mkdir, realpath } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import { resolve, relative, isAbsolute, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { build } from 'vite'

const web = dirname(fileURLToPath(import.meta.url))
if (!process.argv[2]) throw Error('Aufruf: node prepare-gear.mjs <entpackter V17-Ordner>')
const root = await realpath(resolve(process.argv[2]))
const rel = relative(resolve(web, '..'), root)
if (!rel.startsWith('..') && !isAbsolute(rel)) throw Error('Das private Paket muss außerhalb des Repositories liegen.')
const out = resolve(root, 'private-ui')
let html = (await readFile(resolve(root, 'V17_Teststand/WotLK_BiS_Gear_Manager.html'), 'utf8')).replace(/^\uFEFF/, '').replaceAll('\r\n', '\n')
if (createHash('sha256').update(html).digest('hex') !== '562dac465e7a25c4f6750af6a476019a916d6e4e741d078e8c63708bd0021d9f') throw Error('Unbekannte V17-Version. Bitte zuerst den Quelltext prüfen.')
// The checksum above pins the template; extract its known application block,
// rather than treating arbitrary HTML as input to a script sanitizer.
const start = html.indexOf('<script>')
const end = html.indexOf('</script>', start)
if (start < 0 || end < 0) throw Error('V17-Anwendungsskript fehlt.')
const script = html.slice(start + '<script>'.length, end)
const adapter = await readFile(resolve(web,'gear-adapter.js'), 'utf8')
const listStyles = await readFile(resolve(web,'gear-list.css'), 'utf8')
const js = script.replace('"wotlk_bis_manager_v1"', '"wotlk_bis_profile:" + document.documentElement.dataset.player + ":" + setup.character.id')
  .replace('return store._characterLooks[activeChar]||{race:1,gender:1};', 'const look=store._characterLooks[activeChar]||{race:1,gender:1}; return {...look,race:raceID||look.race};')
  .replace(/function createCharacter\(\)\{[\s\S]*?\n\}/, '')
  .replace('byId("deletechar").disabled=!activeChar;', '')
  .replace(/byId\("export"\)\.addEventListener[\s\S]*?(?=\{const ph=byId\("phase"\))/, '')
  .replace(' Bitte exportiere jetzt ein Backup und öffne die Datei möglichst über denselben Browser-Speicherort.', '')
  .replace(/byId\("newchar"\)\.addEventListener[\s\S]*?(?=byId\("phase"\)\.addEventListener)/, '')
  .replace('const byId=', 'const wished = new Set(setup.wishes), saving = new Set(); const byId=')
  .replace('const label=document.createElement("label");label.className="item-choice"', 'const label=document.createElement("div");label.className="item-choice"')
  .replace('actions.append(idButton,box);', 'const toggle=document.createElement("label");toggle.className="owned-toggle";toggle.append(box,"Vorhanden");actions.append(idButton,toggle);')
  .replace('hasAny?"TEIL VORHANDEN":"FEHLT"', 'hasAny?"Vorhanden":"Offen"')
  .replace(/meta\.textContent=\(mapped\?[^\n]+?copy\.append\(meta\);/, 'meta.textContent=[source?.boss || source?.sourceLabel || source?.raid,source?.size ? source.size+"er" : ""].filter(Boolean).join(" · ");if(meta.textContent)copy.append(meta);')
  .replace(/const makeRgLink=[\s\S]*?(?=const idButton=)/, `const link=document.createElement('a');link.textContent=name;link.target='_blank';link.rel='noreferrer';
    const rgID=mapped?.id || (candidates.length===1 && candidates[0].id);
    link.href='https://db.rising-gods.de/?'+(rgID?'item='+encodeURIComponent(rgID):'search='+encodeURIComponent(name));link.title='In der Rising-Gods-Datenbank öffnen';copy.firstChild.replaceWith(link);
    `)
  .replace('rgLinks.forEach(a=>actions.append(a));', '')
  .replace(/const foot=document\.createElement\("div"\);foot\.className="source-note";[^\n]+/, '')
  .replace(/refreshCharacters\(\);render\(\);\s*$/, () => adapter)
  .replaceAll("location.origin+'/data/", "location.origin+'/api/v1/gear-manager/data/")
  .replace('https://esm.sh/wotlk-model-viewer@1.1.8', './viewer.js')
  .replace('Viewer-Bibliothek laden (esm.sh)', 'Viewer-Bibliothek laden')
  // The optional WarcraftDB lookup and external icon overrides aren't part of
  // the offline workflow. Keep the original local icon mapping and RG links.
  .replace(/const idButton=document\.createElement\("button"\);[^\n]+/, 'const idButton=checked ? document.createElement("span") : wishControls(name,candidates,source,mapped);')
  .replace(/function iconUrl\(icon\)\{[\s\S]*?\n\}/, 'function iconUrl(){return "";}')
const filters = html.match(/<section class="filterbar">[\s\S]*?<\/section>/)[0]
html = html.replace(filters, '').replace('<div class="layout">', () => filters + '\n<div class="layout">')
  .replace(/^ *<button class="subtle" id="(?:import|export)">.*\n/gm, '')
  .replace(' Nutze regelmäßig „Backup exportieren“.', '')
  .replace(/<link rel="stylesheet"[\s\S]*?<style>/, '<script src="jquery.js"></script><style>')
  .replace(/<div [^>]*><button [^>]*id="newchar"[^>]*>[^\n]*?<\/div>/, '')
  .replace('<html lang="de">', '<html lang="de" data-player="PLAYER_ID">')
  .replace(`<script>${script}</script>`, '<script src="planner.js"></script>')
  .replace('<header>', '<p id="wish-status" role="status"></p><header>')
  .replace('</style>', `
    :root{--bg:#080f1c;--panel:#101d2e;--panel2:#16273b;--line:#2a4057;--gold:#86d9f2;--text:#dce6f0;--muted:#9eafc3;color-scheme:dark}
    html{overflow-y:hidden}body{display:flow-root;background:#0f1a2b;color:var(--text)} header,footer,.toolbar>div:has(#charSelect){display:none}
    main{padding:12px}.toolbar,.filterbar,.summary,.card,.doll-panel,.info-box{background:#101d2e;border-color:#2a4057;border-radius:4px}
    button,select,input{border-color:#5989a3;color:#dce6f0;background:#16273b} h2,h3,.slot,.stat strong{color:#86d9f2}
    .toolbar{position:static;grid-template-columns:repeat(auto-fit,minmax(180px,1fr))}#wish-status:not(:empty){padding:8px 16px;color:#c7efff;background:#0c2a3c}
    ${listStyles}</style>`)
  .replace('Viewer-Bibliothek wird online geladen', 'Viewer und Modelldaten vom Gildenserver')
const jq = await fetch('https://code.jquery.com/jquery-3.7.1.min.js')
if (!jq.ok) throw Error('jQuery konnte nicht geladen werden.')
const bytes = Buffer.from(await jq.arrayBuffer())
if (createHash('sha256').update(bytes).digest('base64') !== '/JqT3SQfawRcv/BIHPThkBvs0OEvtFFmqPF/lYI/Cxo=') throw Error('jQuery-Prüfsumme stimmt nicht.')
await mkdir(out, {recursive:true})
await build({configFile:false, root:web, publicDir:false, build:{outDir:out, emptyOutDir:false,
  lib:{entry:resolve(web,'node_modules/wotlk-model-viewer/index.js'), formats:['es'], fileName:()=>'viewer.js'},
  rolldownOptions:{output:{codeSplitting:false}}}})
await writeFile(resolve(out,'jquery.js'),bytes)
await writeFile(resolve(out,'planner.js'), `if (window === window.top) location.replace('/#/gear-manager');\nwindow.startGearPlanner = function(setup){\n${js}\n};`)
await writeFile(resolve(out,'index.html'),html)
