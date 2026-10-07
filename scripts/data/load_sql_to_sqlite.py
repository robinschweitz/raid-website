#!/usr/bin/env python3
"""
Load AzerothCore SQL files into SQLite and build the loot catalog.

This script:
1. Parses SQL files from the azerothcore/database-wotlk repo
2. Loads them into a SQLite database
3. Reads item, boss, and loot data
4. Fetches item stats from Wowhead tooltip API
5. Writes the complete catalog JSON

Usage:
    python scripts/data/load_sql_to_sqlite.py [--repo-path scripts/data/azerothcore-db]
    python scripts/data/build_catalog.py --db-type sqlite --db-path scripts/data/azerothcore.db
"""

import argparse
import re
import sqlite3
import sys
from pathlib import Path

# SQL files to load (in order)
SQL_FILES = [
    "sql/base/item_template.sql",
    "sql/base/item_template_locale.sql",
    "sql/base/creature_template.sql",
    "sql/base/creature_template_locale.sql",
    "sql/base/creature.sql",
    "sql/base/creature_loot_template.sql",
    "sql/base/gameobject_template.sql",
    "sql/base/gameobject.sql",
    "sql/base/gameobject_loot_template.sql",
    "sql/base/item_set_names.sql",
    "sql/base/locales_item.sql",
    "sql/base/locales_item_set_names.sql",
]


def parse_sql_file(content: str) -> tuple[dict[str, list[str]], dict[str, list[list]]]:
    """Parse SQL file and return table schemas and data."""
    schemas = {}
    data = {}

    # Parse CREATE TABLE statements
    create_pattern = r"CREATE TABLE `(\w+)`\s*\((.*?)\)\s*(?:ENGINE|;)"
    for match in re.finditer(create_pattern, content, re.DOTALL):
        table = match.group(1)
        cols_str = match.group(2)
        columns = []
        for line in cols_str.split('\n'):
            line = line.strip()
            col_match = re.match(r"`(\w+)`", line)
            if col_match:
                columns.append(col_match.group(1))
        if columns:
            schemas[table] = columns

    # Parse INSERT INTO statements (without column names)
    insert_pattern = r"INSERT INTO `(\w+)` VALUES\s+(.*?);"
    for match in re.finditer(insert_pattern, content, re.DOTALL):
        table = match.group(1)
        values_str = match.group(2)

        if table not in data:
            data[table] = []

        # Parse value tuples
        rows = []
        current = ""
        depth = 0
        for char in values_str:
            if char == '(':
                depth += 1
                if depth == 1:
                    current = ""
                    continue
            elif char == ')':
                depth -= 1
                if depth == 0:
                    rows.append(current)
                    current = ""
                    continue
            if depth > 0:
                current += char

        for row in rows:
            # Split by comma, respecting quotes
            values = []
            in_quote = False
            current_val = ""
            i = 0
            while i < len(row):
                char = row[i]
                if char == "'" and (i == 0 or row[i-1] != '\\'):
                    in_quote = not in_quote
                    current_val += char
                elif char == ',' and not in_quote:
                    values.append(current_val.strip())
                    current_val = ""
                else:
                    current_val += char
                i += 1
            if current_val:
                values.append(current_val.strip())

            # Clean up values
            cleaned = []
            for v in values:
                if v.startswith("'") and v.endswith("'"):
                    cleaned.append(v[1:-1].replace("\\'", "'").replace('\\"', '"'))
                elif v == "NULL":
                    cleaned.append(None)
                else:
                    try:
                        if '.' in v:
                            cleaned.append(float(v))
                        else:
                            cleaned.append(int(v))
                    except ValueError:
                        cleaned.append(v)

            if table in schemas and len(cleaned) == len(schemas[table]):
                data[table].append(cleaned)

    return schemas, data


def load_sql_files(repo_path: Path, db_path: Path):
    """Load all SQL files into SQLite database."""
    conn = sqlite3.connect(db_path)
    cursor = conn.cursor()

    for sql_file in SQL_FILES:
        file_path = repo_path / sql_file
        if not file_path.exists():
            print(f"  [skip] {sql_file} not found")
            continue

        print(f"  Loading {sql_file}...")
        content = file_path.read_text(encoding='utf-8', errors='ignore')

        schemas, data = parse_sql_file(content)

        for table, columns in schemas.items():
            # Drop and recreate table
            cursor.execute(f'DROP TABLE IF EXISTS "{table}"')
            col_defs = ", ".join(f'"{c}"' for c in columns)
            cursor.execute(f'CREATE TABLE "{table}" ({col_defs})')

            # Insert data
            if table in data:
                placeholders = ", ".join("?" for _ in columns)
                col_names = ", ".join(f'"{c}"' for c in columns)
                for row in data[table]:
                    cursor.execute(f'INSERT INTO "{table}" ({col_names}) VALUES ({placeholders})', row)

                print(f"    {table}: {len(data[table])} rows")

    conn.commit()
    conn.close()
    print(f"Database saved: {db_path}")


def main():
    parser = argparse.ArgumentParser(description="Load AzerothCore SQL into SQLite")
    parser.add_argument("--repo-path", type=str, default="scripts/data/azerothcore-db")
    parser.add_argument("--db-path", type=str, default="scripts/data/azerothcore.db")
    args = parser.parse_args()

    repo_path = Path(args.repo_path)
    db_path = Path(args.db_path)

    if not repo_path.exists():
        print(f"Repo not found: {repo_path}", file=sys.stderr)
        print("Clone it with: git clone --depth 1 https://github.com/azerothcore/database-wotlk.git scripts/data/azerothcore-db", file=sys.stderr)
        sys.exit(1)

    load_sql_files(repo_path, db_path)


if __name__ == "__main__":
    main()
