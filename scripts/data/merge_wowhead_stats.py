#!/usr/bin/env python3
"""
Merge Wowhead WotLK item stats into the bundled loot catalog.

Uses the nether.wowhead.com tooltip API (XML feeds are blocked by CloudFront).

Usage:
    python scripts/data/merge_wowhead_stats.py [--delay 0.3] [--retries 3]

Reads:  scripts/data/catalog-wotlk335.json
Writes: scripts/data/catalog-wotlk335.json (in-place, with stats fields added)

The script is idempotent: items that already have stats are skipped.
"""

import argparse
import json
import re
import sys
import time
from pathlib import Path

import requests

CATALOG_PATH = Path(__file__).parent / "catalog-wotlk335.json"
TOOLTIP_API = "https://nether.wowhead.com/wotlk/tooltip/item/{item_id}"

# Map Wowhead English stat names to our JSON fields
STAT_MAP = {
    "Strength": "strength",
    "Agility": "agility",
    "Stamina": "stamina",
    "Intellect": "intellect",
    "Spirit": "spirit",
    "Spell Power": "spell_power",
    "Armor": "armor",
}

# Map Wowhead English slot names to our JSON fields
SLOT_MAP = {
    "Head": "Head",
    "Neck": "Neck",
    "Shoulder": "Shoulder",
    "Shirt": "Shirt",
    "Chest": "Chest",
    "Waist": "Waist",
    "Legs": "Legs",
    "Feet": "Feet",
    "Wrist": "Wrist",
    "Hands": "Hands",
    "Finger": "Finger",
    "Trinket": "Trinket",
    "Weapon": "Weapon",
    "Shield": "Shield",
    "Ranged": "Ranged",
    "Cloak": "Cloak",
    "Two-Hand": "Two-Hand",
    "Bag": "Bag",
    "Tabard": "Tabard",
    "Robe": "Robe",
    "Main Hand": "Main Hand",
    "Off Hand": "Off Hand",
    "Thrown": "Thrown",
    "Quiver": "Quiver",
    "Relic": "Relic",
    "Ammo": "Ammo",
}


def fetch_tooltip(session: requests.Session, item_id: str, retries: int, delay: float) -> dict | None:
    """Fetch Wowhead tooltip JSON for an item, with retries."""
    url = TOOLTIP_API.format(item_id=item_id)
    for attempt in range(retries):
        try:
            resp = session.get(url, timeout=30)
            if resp.status_code == 200:
                return resp.json()
            if resp.status_code == 404:
                print(f"  [404] Item {item_id} not found", file=sys.stderr)
                return None
            if resp.status_code == 429:
                wait = delay * (2 ** attempt)
                print(f"  [429] Rate limited, waiting {wait:.1f}s", file=sys.stderr)
                time.sleep(wait)
                continue
            print(f"  [{resp.status_code}] Item {item_id}", file=sys.stderr)
        except requests.RequestException as e:
            print(f"  [error] Item {item_id}: {e}", file=sys.stderr)
        if attempt < retries - 1:
            time.sleep(delay * (2 ** attempt))
    return None


def parse_tooltip(tooltip: dict) -> dict:
    """Parse Wowhead tooltip JSON and extract stats into our JSON format."""
    stats = {}

    html = tooltip.get("tooltip", "")

    # Item level: "Item Level <!--ilvl-->264"
    ilvl_match = re.search(r"Item Level\s+(?:<!--ilvl-->)?(\d+)", html)
    if ilvl_match:
        stats["item_level"] = int(ilvl_match.group(1))

    # Slot: "<td>Wrist</td>" in the slot table
    slot_match = re.search(r"<td>([A-Za-z-]+)</td>", html)
    if slot_match:
        slot_name = slot_match.group(1).strip()
        stats["slot"] = SLOT_MAP.get(slot_name, slot_name)

    # Armor: "<!--amr-->1155 Armor"
    armor_match = re.search(r"(?:<!--amr-->)?(\d+)\s+Armor", html)
    if armor_match:
        stats["armor"] = int(armor_match.group(1))

    # Stats: "<!--stat4-->+90 Strength", "<!--stat7-->+124 Stamina"
    for stat_name, field in STAT_MAP.items():
        pattern = rf"<!--stat\d+-->\+(\d+)\s+{re.escape(stat_name)}"
        match = re.search(pattern, html)
        if match:
            stats[field] = int(match.group(1))

    # Damage: "234 - 351 Damage" (weapons)
    damage_match = re.search(r"(\d+(?:\.\d+)?)\s*-\s*(\d+(?:\.\d+)?)\s+Damage", html)
    if damage_match:
        stats["min_damage"] = float(damage_match.group(1))
        stats["max_damage"] = float(damage_match.group(2))

    # Speed: "Speed 2.60"
    speed_match = re.search(r"Speed\s+(\d+(?:\.\d+)?)", html)
    if speed_match and "min_damage" in stats and "max_damage" in stats:
        speed = float(speed_match.group(1))
        if speed > 0:
            stats["dps"] = round(
                (stats["min_damage"] + stats["max_damage"]) / 2.0 / speed, 2
            )

    return stats


def main():
    parser = argparse.ArgumentParser(description="Merge Wowhead stats into catalog")
    parser.add_argument("--delay", type=float, default=0.3, help="Delay between requests (seconds)")
    parser.add_argument("--retries", type=int, default=3, help="Retries per item")
    args = parser.parse_args()

    if not CATALOG_PATH.exists():
        print(f"Catalog not found: {CATALOG_PATH}", file=sys.stderr)
        sys.exit(1)

    with open(CATALOG_PATH, "r", encoding="utf-8") as f:
        catalog = json.load(f)

    items = catalog.get("items", [])
    total = len(items)
    print(f"Processing {total} items...")

    session = requests.Session()
    session.headers.update({
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
    })

    updated = 0
    skipped = 0
    failed = 0

    for i, item in enumerate(items, 1):
        item_id = item.get("item_id", "")
        if not item_id:
            continue

        # Skip if already has stats
        if item.get("item_level") or item.get("slot") or item.get("armor"):
            skipped += 1
            continue

        if i % 50 == 0:
            print(f"  Progress: {i}/{total} (updated={updated}, skipped={skipped}, failed={failed})")

        tooltip = fetch_tooltip(session, item_id, args.retries, args.delay)
        if tooltip is None:
            failed += 1
            continue

        stats = parse_tooltip(tooltip)
        if stats:
            item.update(stats)
            updated += 1

        time.sleep(args.delay)

    print(f"\nDone: {updated} updated, {skipped} skipped, {failed} failed")

    # Write back
    with open(CATALOG_PATH, "w", encoding="utf-8") as f:
        json.dump(catalog, f, ensure_ascii=False, indent=2)

    print(f"Written: {CATALOG_PATH}")


if __name__ == "__main__":
    main()
