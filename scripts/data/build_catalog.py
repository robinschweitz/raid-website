#!/usr/bin/env python3
"""
Build the complete WotLK loot catalog from AzerothCore SQL + Wowhead stats.

This script:
1. Reads item, boss, and loot data from AzerothCore SQL (SQLite or MySQL)
2. Fetches item stats from the Wowhead tooltip API
3. Writes a complete catalog JSON file

Usage:
    python scripts/data/build_catalog.py --db-type sqlite --db-path /path/to/azerothcore.db
    python scripts/data/build_catalog.py --db-type mysql --db-host localhost --db-user root --db-password pass --db-name acore

Requires: requests, mysql-connector-python (for MySQL)
"""

import argparse
import json
import re
import sys
import time
from pathlib import Path

import requests

# WotLK raid instances with their AzerothCore map IDs
# Map IDs: 533=Naxxramas, 603=Ulduar, 624=Archavon, 631=ICC, 649=ToC, 724=Ruby Sanctum
WOTLK_INSTANCES = [
    {"id": "533", "name": "Naxxramas", "map_id": 533},
    {"id": "603", "name": "Ulduar", "map_id": 603},
    {"id": "624", "name": "Archavons Kammer", "map_id": 624},
    {"id": "631", "name": "Eiskronenzitadelle", "map_id": 631},
    {"id": "649", "name": "Prüfung des Kreuzritters", "map_id": 649},
    {"id": "724", "name": "Rubinheiligtum", "map_id": 724},
]

TOOLTIP_API = "https://nether.wowhead.com/wotlk/tooltip/item/{item_id}"

# Map AzerothCore stat types to our JSON fields
# AzerothCore stat_type: 3=Agility, 4=Strength, 5=Intellect, 6=Spirit, 7=Stamina
STAT_TYPE_MAP = {
    3: "agility",
    4: "strength",
    5: "intellect",
    6: "spirit",
    7: "stamina",
}

# Map AzerothCore inventory types to slot names
INVENTORY_TYPE_MAP = {
    1: "Head", 2: "Neck", 3: "Shoulder", 4: "Shirt", 5: "Chest",
    6: "Waist", 7: "Legs", 8: "Feet", 9: "Wrist", 10: "Hands",
    11: "Finger", 12: "Trinket", 13: "Weapon", 14: "Shield",
    15: "Ranged", 16: "Cloak", 17: "Two-Hand", 18: "Bag",
    19: "Tabard", 20: "Robe", 21: "Main Hand", 22: "Off Hand",
    23: "Holdable", 24: "Ammo", 25: "Thrown", 26: "Ranged",
    27: "Quiver", 28: "Relic",
}


def connect_sqlite(db_path: str):
    """Connect to SQLite database."""
    import sqlite3
    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    return conn


def connect_mysql(host: str, user: str, password: str, database: str, port: int = 3306):
    """Connect to MySQL database."""
    import mysql.connector
    return mysql.connector.connect(
        host=host, user=user, password=password, database=database, port=port
    )


def fetch_items_sqlite(conn) -> list[dict]:
    """Fetch item data from SQLite."""
    cursor = conn.cursor()
    cursor.execute("""
        SELECT entry, name, bonding, quality, ItemLevel, InventoryType, armor,
               stat_type1, stat_value1, stat_type2, stat_value2, stat_type3, stat_value3,
               stat_type4, stat_value4, stat_type5, stat_value5, stat_type6, stat_value6,
               stat_type7, stat_value7, stat_type8, stat_value8, stat_type9, stat_value9,
               stat_type10, stat_value10, dmg_min1, dmg_max1, delay,
               subclass, class
        FROM item_template
    """)
    items = []
    for row in cursor.fetchall():
        item = {
            "item_id": str(row["entry"]),
            "name": row["name"],
            "binding": row["bonding"],
            "quality": row["quality"],
        }
        if row["ItemLevel"] and row["ItemLevel"] > 0:
            item["item_level"] = row["ItemLevel"]
        if row["InventoryType"] in INVENTORY_TYPE_MAP:
            item["slot"] = INVENTORY_TYPE_MAP[row["InventoryType"]]
        if row["armor"] and row["armor"] > 0:
            item["armor"] = row["armor"]

        # Stats
        for i in range(1, 11):
            stat_type = row[f"stat_type{i}"]
            stat_value = row[f"stat_value{i}"]
            if stat_type and stat_value and stat_type in STAT_TYPE_MAP:
                field = STAT_TYPE_MAP[stat_type]
                if field not in item:
                    item[field] = stat_value

        # Damage
        if row["dmg_min1"] and row["dmg_max1"] and row["dmg_min1"] > 0:
            item["min_damage"] = float(row["dmg_min1"])
            item["max_damage"] = float(row["dmg_max1"])
            if row["delay"] and row["delay"] > 0:
                speed = float(row["delay"]) / 1000.0
                item["dps"] = round((item["min_damage"] + item["max_damage"]) / 2.0 / speed, 2)

        if row["subclass"] is not None:
            item["subclass"] = row["subclass"]
        if row["class"] is not None:
            item["class_mask"] = row["class"]

        items.append(item)
    return items


def fetch_items_mysql(conn) -> list[dict]:
    """Fetch item data from MySQL."""
    cursor = conn.cursor(dictionary=True)
    cursor.execute("""
        SELECT entry, name, bonding, quality, itemlevel, inventorytype, armor,
               stat_type1, stat_value1, stat_type2, stat_value2, stat_type3, stat_value3,
               stat_type4, stat_value4, stat_type5, stat_value5, stat_type6, stat_value6,
               stat_type7, stat_value7, stat_type8, stat_value8, stat_type9, stat_value9,
               stat_type10, stat_value10, dmg_min1, dmg_max1, delay, spellpower,
               subclass, class
        FROM item_template
    """)
    items = []
    for row in cursor.fetchall():
        item = {
            "item_id": str(row["entry"]),
            "name": row["name"],
            "binding": row["bonding"],
            "quality": row["quality"],
        }
        if row["itemlevel"] and row["itemlevel"] > 0:
            item["item_level"] = row["itemlevel"]
        if row["inventorytype"] in INVENTORY_TYPE_MAP:
            item["slot"] = INVENTORY_TYPE_MAP[row["inventorytype"]]
        if row["armor"] and row["armor"] > 0:
            item["armor"] = row["armor"]
        if row["spellpower"] and row["spellpower"] > 0:
            item["spell_power"] = row["spellpower"]

        for i in range(1, 11):
            stat_type = row[f"stat_type{i}"]
            stat_value = row[f"stat_value{i}"]
            if stat_type and stat_value and stat_type in STAT_TYPE_MAP:
                field = STAT_TYPE_MAP[stat_type]
                if field not in item:
                    item[field] = stat_value

        if row["dmg_min1"] and row["dmg_max1"] and row["dmg_min1"] > 0:
            item["min_damage"] = float(row["dmg_min1"])
            item["max_damage"] = float(row["dmg_max1"])
            if row["delay"] and row["delay"] > 0:
                speed = float(row["delay"]) / 1000.0
                item["dps"] = round((item["min_damage"] + item["max_damage"]) / 2.0 / speed, 2)

        if row["subclass"] is not None:
            item["subclass"] = row["subclass"]
        if row["class"] is not None:
            item["class_mask"] = row["class"]

        items.append(item)
    return items


def fetch_bosses_and_drops(conn, db_type: str) -> tuple[list[dict], list[dict]]:
    """Fetch bosses and their loot drops from the database."""
    cursor = conn.cursor()

    # Get all bosses (creatures) in WotLK raid instances
    map_ids = [str(inst["map_id"]) for inst in WOTLK_INSTANCES]
    placeholders = ",".join(["?" if db_type == "sqlite" else "%s"] * len(map_ids))

    cursor.execute(f"""
        SELECT ct.entry, ct.name, c.map
        FROM creature_template ct
        JOIN creature c ON c.id = ct.entry
        WHERE c.map IN ({placeholders})
        AND ct.rank >= 1
    """, map_ids)

    bosses = []
    for row in cursor.fetchall():
        entry = row[0] if db_type == "sqlite" else row["entry"]
        name = row[1] if db_type == "sqlite" else row["name"]
        map_id = row[2] if db_type == "sqlite" else row["map"]
        bosses.append({"id": str(entry), "name": name, "map_id": map_id})

    # Get loot templates
    drops = []
    for boss in bosses:
        cursor.execute("""
            SELECT item, ChanceOrQuestChance, lootmode, groupid, mincountOrRef, maxcount
            FROM creature_loot_template
            WHERE Entry = ?
        """, (boss["id"],))

        for row in cursor.fetchall():
            item_id = row[0] if db_type == "sqlite" else row["item"]
            chance = row[1] if db_type == "sqlite" else row["ChanceOrQuestChance"]
            lootmode = row[2] if db_type == "sqlite" else row["lootmode"]
            groupid = row[3] if db_type == "sqlite" else row["groupid"]
            mincount = row[4] if db_type == "sqlite" else row["mincountOrRef"]
            maxcount = row[5] if db_type == "sqlite" else row["maxcount"]

            drops.append({
                "boss_id": boss["id"],
                "item_id": str(item_id),
                "chance": chance,
                "lootmode": lootmode,
                "groupid": groupid,
                "mincount": mincount,
                "maxcount": maxcount,
            })

    return bosses, drops


def fetch_tooltip(session: requests.Session, item_id: str, retries: int, delay: float) -> dict | None:
    """Fetch Wowhead tooltip JSON for an item."""
    url = TOOLTIP_API.format(item_id=item_id)
    for attempt in range(retries):
        try:
            resp = session.get(url, timeout=30)
            if resp.status_code == 200:
                return resp.json()
            if resp.status_code == 404:
                return None
            if resp.status_code == 429:
                wait = delay * (2 ** attempt)
                time.sleep(wait)
                continue
        except requests.RequestException:
            pass
        if attempt < retries - 1:
            time.sleep(delay * (2 ** attempt))
    return None


def parse_tooltip(tooltip: dict) -> dict:
    """Parse Wowhead tooltip JSON and extract stats."""
    stats = {}
    html = tooltip.get("tooltip", "")

    ilvl_match = re.search(r"Item Level\s+(?:<!--ilvl-->)?(\d+)", html)
    if ilvl_match:
        stats["item_level"] = int(ilvl_match.group(1))

    slot_match = re.search(r"<td>([A-Za-z-]+)</td>", html)
    if slot_match:
        stats["slot"] = slot_match.group(1).strip()

    armor_match = re.search(r"(?:<!--amr-->)?(\d+)\s+Armor", html)
    if armor_match:
        stats["armor"] = int(armor_match.group(1))

    for stat_name, field in [("Strength", "strength"), ("Agility", "agility"),
                             ("Stamina", "stamina"), ("Intellect", "intellect"),
                             ("Spirit", "spirit"), ("Spell Power", "spell_power")]:
        pattern = rf"<!--stat\d+-->\+(\d+)\s+{re.escape(stat_name)}"
        match = re.search(pattern, html)
        if match:
            stats[field] = int(match.group(1))

    damage_match = re.search(r"(\d+(?:\.\d+)?)\s*-\s*(\d+(?:\.\d+)?)\s+Damage", html)
    if damage_match:
        stats["min_damage"] = float(damage_match.group(1))
        stats["max_damage"] = float(damage_match.group(2))

    speed_match = re.search(r"Speed\s+(\d+(?:\.\d+)?)", html)
    if speed_match and "min_damage" in stats and "max_damage" in stats:
        speed = float(speed_match.group(1))
        if speed > 0:
            stats["dps"] = round((stats["min_damage"] + stats["max_damage"]) / 2.0 / speed, 2)

    return stats


def main():
    parser = argparse.ArgumentParser(description="Build WotLK loot catalog from AzerothCore SQL")
    parser.add_argument("--db-type", choices=["sqlite", "mysql"], required=True)
    parser.add_argument("--db-path", type=str, help="Path to SQLite database")
    parser.add_argument("--db-host", type=str, default="localhost")
    parser.add_argument("--db-user", type=str, default="root")
    parser.add_argument("--db-password", type=str, default="")
    parser.add_argument("--db-name", type=str, default="acore")
    parser.add_argument("--db-port", type=int, default=3306)
    parser.add_argument("--delay", type=float, default=0.3, help="Delay between Wowhead requests")
    parser.add_argument("--retries", type=int, default=3)
    parser.add_argument("--output", type=str, default="scripts/data/catalog-wotlk335.json")
    parser.add_argument("--skip-stats", action="store_true", help="Skip Wowhead stats fetching")
    args = parser.parse_args()

    # Connect to database
    if args.db_type == "sqlite":
        if not args.db_path:
            print("Error: --db-path required for SQLite", file=sys.stderr)
            sys.exit(1)
        conn = connect_sqlite(args.db_path)
        items = fetch_items_sqlite(conn)
    else:
        conn = connect_mysql(args.db_host, args.db_user, args.db_password, args.db_name, args.db_port)
        items = fetch_items_mysql(conn)

    print(f"Fetched {len(items)} items from database")

    # Fetch bosses and drops
    bosses, drops = fetch_bosses_and_drops(conn, args.db_type)
    print(f"Fetched {len(bosses)} bosses and {len(drops)} drops")

    # Fetch stats from Wowhead (optional)
    if not args.skip_stats:
        print("\nFetching stats from Wowhead...")
        session = requests.Session()
        session.headers.update({"User-Agent": "Mozilla/5.0"})

        unique_items = {d["item_id"] for d in drops}
        print(f"Unique items in drops: {len(unique_items)}")

        item_stats = {}
        for i, item_id in enumerate(unique_items, 1):
            if i % 50 == 0:
                print(f"  Progress: {i}/{len(unique_items)}")
            tooltip = fetch_tooltip(session, item_id, args.retries, args.delay)
            if tooltip:
                stats = parse_tooltip(tooltip)
                if stats:
                    item_stats[item_id] = stats
            time.sleep(args.delay)

        print(f"Items with stats: {len(item_stats)}")

        # Merge stats into items
        for item in items:
            if item["item_id"] in item_stats:
                item.update(item_stats[item["item_id"]])

    # Build catalog
    catalog = {
        "source": {
            "id": "azerothcore-wotlk",
            "revision": "unknown",
            "url": "https://github.com/azerothcore/azerothcore-wotlk",
            "license": "GPL-2.0",
            "attribution": "AzerothCore contributors",
        },
        "notices": [
            f"Built from AzerothCore SQL on {time.strftime('%Y-%m-%d')}",
            f"{len(bosses)} bosses, {len(drops)} drops, {len(items)} items",
        ],
        "instances": [{"id": inst["id"], "name": inst["name"]} for inst in WOTLK_INSTANCES],
        "bosses": [{"id": b["id"], "instance_id": str(b["map_id"]), "name": b["name"]} for b in bosses],
        "items": items,
        "drops": drops,
    }

    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(catalog, f, ensure_ascii=False, indent=2)

    print(f"\nWritten: {output_path}")
    print(f"  Instances: {len(catalog['instances'])}")
    print(f"  Bosses: {len(catalog['bosses'])}")
    print(f"  Items: {len(catalog['items'])}")
    print(f"  Drops: {len(catalog['drops'])}")


if __name__ == "__main__":
    main()
