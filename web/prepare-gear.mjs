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
const js = script.replace('"wotlk_bis_manager_v1"', '"wotlk_bis_manager_v1:" + document.documentElement.dataset.player')
  .replaceAll("location.origin+'/data/", "location.origin+'/api/v1/gear-manager/data/")
  .replace('https://esm.sh/wotlk-model-viewer@1.1.8', './viewer.js')
  .replace('Viewer-Bibliothek laden (esm.sh)', 'Viewer-Bibliothek laden')
  // The optional WarcraftDB lookup and external icon overrides aren't part of
  // the offline workflow. Keep the original local icon mapping and RG links.
  .replace(/const idButton=document\.createElement\("button"\);[^\n]+/, 'const idButton=document.createElement("span");')
  .replace(/function iconUrl\(icon\)\{[\s\S]*?\n\}/, 'function iconUrl(){return "";}')
html = html.replace(/<link rel="stylesheet"[\s\S]*?<style>/, '<script src="jquery.js"></script><style>')
  .replace('<html lang="de">', '<html lang="de" data-player="PLAYER_ID">')
  .replace(`<script>${script}</script>`, '<script src="planner.js"></script>')
  .replace('<header>', '<header><a href="/" style="color:inherit">← Zur Raid-Website</a>')
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
await writeFile(resolve(out,'planner.js'),js)
await writeFile(resolve(out,'index.html'),html)
