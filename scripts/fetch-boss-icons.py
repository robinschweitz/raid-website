"""Download the dashboard's Blizzard boss artwork from Wowhead's CDN."""
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
import re
import urllib.request
import urllib.parse

JOURNAL = {
    'anubrekan': 'AnubRekhan',
    'instructorrazuvious': 'Instructor Razuvious',
    'noththeplaguebringer': 'Noth the Plaguebringer',
    'gothiktheharvester': 'Gothik the Harvester',
    'grobbulus': 'Grobbulus', 'gluth': 'Gluth',
    'archavon': 'Archavon the Stone Watcher',
    'emalon_01': 'Emalon the Storm Watcher',
    'koralon': 'Koralon the Flame Watcher',
    'lordjaraxxus': 'Lord Jaraxxus',
    'gormoktheimpaler': 'Northrend Beasts',
    'twinvalkyr': 'Twin Valkyr', 'halion': 'Halion',
}
ICONS = {
    'thefourhorsemen': 'achievement_boss_fourhorsemen',
    'sapphiron': 'achievement_boss_sapphiron_01',
    'flameleviathan_01': 'achievement_boss_theflameleviathan_01',
    'theironcouncil_01': 'achievement_boss_theironcouncil_01',
    'gunshipbattle': 'achievement_boss_skybreaker',
    'festergut': 'achievement_boss_festergutrotface',
    'rotface': 'achievement_boss_festergutrotface',
    'bloodqueenlanathel': 'achievement_boss_lanathel',
    'bloodprincecouncil': 'achievement_boss_valanar',
    'valithriadreamwalker': 'achievement_boss_valithradreamwalker',
    'factionchampions': 'achievement_arena_5v5_7',
    'toravon': 'spell_frost_frozencore',
    'baltharus': 'inv_misc_head_dragon_01',
    'saviana': 'ability_mount_drake_red',
    'zarithrian': 'inv_misc_head_dragon_01',
}

ROOT = Path(__file__).resolve().parents[1]
DEST = ROOT / 'web/public/bosses'

def fetch(icon):
    from PIL import Image
    from io import BytesIO
    if icon in JOURNAL:
        path = urllib.parse.quote(f'EncounterJournal/UI-EJ-BOSS-{JOURNAL[icon]}.PNG')
        url = f'https://raw.githubusercontent.com/Gethe/wow-ui-textures/live/{path}'
    else:
        name = ICONS.get(icon, f'achievement_boss_{icon}')
        url = f'https://wow.zamimg.com/images/wow/icons/large/{name}.jpg'
    try:
        with urllib.request.urlopen(url, timeout=30) as response:
            data = response.read()
        image = Image.open(BytesIO(data)).convert('RGBA')
        image.thumbnail((128, 128))
        image.save(DEST / f'{icon}.webp')
        (DEST / f'{icon}.jpg').unlink(missing_ok=True)
        return True
    except Exception as error:
        print(f'{icon}: {error}')
        return False

if __name__ == '__main__':
    DEST.mkdir(exist_ok=True)
    source = (ROOT / 'web/src/components/bossData.ts').read_text(encoding='utf-8')
    icons = re.findall(r", '([^']+)'\)", source)
    with ThreadPoolExecutor(max_workers=6) as pool:
        results = list(pool.map(fetch, icons))
    print(f'{sum(results)}/{len(results)} icons downloaded')
    raise SystemExit(0 if all(results) else 1)
