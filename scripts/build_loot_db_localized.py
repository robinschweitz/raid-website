#!/usr/bin/env python3
"""
Build a multilingual WotLK loot database from AtlasLoot 3.3.5a + Wowhead.

Data model
----------
AtlasLoot:
    instance -> stable loot-table IDs -> boss/source -> item IDs/drop rates

Wowhead (English):
    canonical item details + parsed numerical stats

Wowhead (other configured locales):
    localized item name + localized tooltip text/html

AtlasLoot LibBabble / locale files:
    localized instance/zone names + boss/source names

Important design rule:
    Stats are parsed ONLY from the English tooltip. Localized tooltips are
    presentation data, so adding a language never changes canonical stats.

Usage:
    python build_loot_db_localized.py config.json
    python build_loot_db_localized.py config.json --refresh

Python:
    3.10+

External dependencies:
    None
"""

from __future__ import annotations

import argparse
import html
import json
import re
import sys
import time
import urllib.error
import urllib.request
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


DEFAULT_REPO = "Gescht/AtlasLoot3.3.5a"
DEFAULT_BRANCH = "main"

# Wowhead tooltip URLs are intentionally isolated here because this is an
# undocumented service and its URL layout may change in the future.
TOOLTIP_API_EN = "https://nether.wowhead.com/wotlk/tooltip/item/{item_id}"
TOOLTIP_API_LOCALIZED = (
    "https://nether.wowhead.com/wotlk/{lang}/tooltip/item/{item_id}"
)

ATLAS_FILES = {
    "dropdown": "AtlasLoot/DefaultFrame/DewDropDown.lua",
    "register_en": "AtlasLoot/TableRegister/loottables.en.lua",
    "loot": "AtlasLoot_WrathoftheLichKing/wrathofthelichking.lua",
    "boss_babble": (
        "AtlasLoot/Libs/LibBabble-Boss-3.0/LibBabble-Boss-3.0.lua"
    ),
    "zone_babble": (
        "AtlasLoot/Libs/LibBabble-Zone-3.0/LibBabble-Zone-3.0.lua"
    ),
}

# AtlasLoot filename suffix + default Wowhead URL language slug.
#
# If Wowhead changes or a particular locale uses a different slug, override it
# in config.json with:
#
#   "wowhead_locale_slugs": {"deDE": "de", ...}
#
LOCALES = {
    "enUS": {"atlas": "en", "wowhead": ""},
    "deDE": {"atlas": "de", "wowhead": "de"},
    "frFR": {"atlas": "fr", "wowhead": "fr"},
    "esES": {"atlas": "es", "wowhead": "es"},
    "esMX": {"atlas": "mx", "wowhead": "es"},
    "ruRU": {"atlas": "ru", "wowhead": "ru"},
    "koKR": {"atlas": "kr", "wowhead": "ko"},
    "zhCN": {"atlas": "cn", "wowhead": "cn"},
    "zhTW": {"atlas": "tw", "wowhead": "tw"},
}

DIFFICULTY_SUFFIXES = (
    "",                # normal / 10 normal
    "HEROIC",          # heroic / 10 heroic
    "25Man",           # 25 normal
    "25ManHEROIC",     # 25 heroic
)

USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
    "AppleWebKit/537.36 (KHTML, like Gecko) "
    "Chrome/140.0 Safari/537.36 "
    "AtlasLootJsonBuilder/2.0"
)


# ---------------------------------------------------------------------------
# HTTP / files
# ---------------------------------------------------------------------------

def http_get_text(url: str, timeout: int = 30, attempts: int = 4) -> str:
    last_error: Exception | None = None

    for attempt in range(1, attempts + 1):
        req = urllib.request.Request(
            url,
            headers={
                "User-Agent": USER_AGENT,
                "Accept": "application/json,text/plain,*/*",
            },
        )
        try:
            with urllib.request.urlopen(req, timeout=timeout) as response:
                return response.read().decode("utf-8-sig", errors="replace")
        except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError) as exc:
            last_error = exc
            if attempt < attempts:
                time.sleep(min(2 ** (attempt - 1), 8))

    raise RuntimeError(f"Failed to fetch {url}: {last_error}")


def atlas_raw_url(config: dict[str, Any], relative_path: str) -> str:
    repo = config.get("atlasloot_repo", DEFAULT_REPO)
    branch = config.get("atlasloot_branch", DEFAULT_BRANCH)
    return f"https://raw.githubusercontent.com/{repo}/{branch}/{relative_path}"


def load_atlas_file(
    config: dict[str, Any],
    relative_path: str,
    *,
    optional: bool = False,
) -> str:
    local_dir = config.get("atlasloot_dir")

    if local_dir:
        path = Path(local_dir) / relative_path
        if not path.exists():
            if optional:
                return ""
            raise FileNotFoundError(f"AtlasLoot file not found: {path}")
        return path.read_text(encoding="utf-8-sig", errors="replace")

    try:
        return http_get_text(atlas_raw_url(config, relative_path))
    except Exception:
        if optional:
            return ""
        raise


# ---------------------------------------------------------------------------
# Lua parsing helpers
# ---------------------------------------------------------------------------

def extract_braced_block(source: str, open_brace_index: int) -> str:
    """Return text inside a Lua {...} block, excluding the outer braces."""
    if source[open_brace_index] != "{":
        raise ValueError("open_brace_index must point at '{'")

    depth = 0
    in_string = False
    quote = ""
    escaped = False
    in_line_comment = False

    for i in range(open_brace_index, len(source)):
        ch = source[i]
        nxt = source[i + 1] if i + 1 < len(source) else ""

        if in_line_comment:
            if ch == "\n":
                in_line_comment = False
            continue

        if in_string:
            if escaped:
                escaped = False
            elif ch == "\\":
                escaped = True
            elif ch == quote:
                in_string = False
            continue

        if ch == "-" and nxt == "-":
            in_line_comment = True
            continue

        if ch in ('"', "'"):
            in_string = True
            quote = ch
            continue

        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                return source[open_brace_index + 1:i]

    raise ValueError("Unterminated Lua table")


def extract_named_table(source: str, name: str) -> str | None:
    pattern = re.compile(r'\["' + re.escape(name) + r'"\]\s*=\s*\{')
    match = pattern.search(source)
    if not match:
        return None
    brace = source.find("{", match.start())
    return extract_braced_block(source, brace)


def parse_all_loot_tables(source: str) -> dict[str, str]:
    result: dict[str, str] = {}
    pattern = re.compile(r'AtlasLoot_Data\["([^"]+)"\]\s*=\s*\{')

    for match in pattern.finditer(source):
        table_id = match.group(1)
        brace = source.find("{", match.start())
        try:
            result[table_id] = extract_braced_block(source, brace)
        except ValueError:
            print(
                f"WARNING: could not parse loot table {table_id}",
                file=sys.stderr,
            )

    return result


def decode_lua_string(raw: str) -> str:
    """
    Decode the subset of Lua string escapes AtlasLoot uses.

    Decimal escapes like \\195\\182 are decoded as bytes and then UTF-8 where
    possible. Normal escaped quotes/backslashes/newlines are also handled.
    """
    out = bytearray()
    unicode_chunks: list[str] = []

    def flush_bytes() -> None:
        nonlocal out
        if not out:
            return
        try:
            unicode_chunks.append(out.decode("utf-8"))
        except UnicodeDecodeError:
            unicode_chunks.append(out.decode("latin-1", errors="replace"))
        out = bytearray()

    i = 0
    while i < len(raw):
        ch = raw[i]

        if ch != "\\":
            flush_bytes()
            unicode_chunks.append(ch)
            i += 1
            continue

        # Decimal Lua byte escape.
        m = re.match(r"\\(\d{1,3})", raw[i:])
        if m:
            out.append(int(m.group(1)) & 0xFF)
            i += len(m.group(0))
            continue

        flush_bytes()

        if i + 1 >= len(raw):
            unicode_chunks.append("\\")
            break

        nxt = raw[i + 1]
        replacements = {
            "n": "\n",
            "r": "\r",
            "t": "\t",
            "\\": "\\",
            '"': '"',
            "'": "'",
        }
        unicode_chunks.append(replacements.get(nxt, nxt))
        i += 2

    flush_bytes()
    return "".join(unicode_chunks)


def first_quoted_string(field: str) -> str:
    m = re.search(r'"((?:\\.|[^"\\])*)"', field)
    if not m:
        return ""
    return decode_lua_string(m.group(1))


def split_lua_fields(text: str) -> list[str]:
    fields: list[str] = []
    buf: list[str] = []
    paren = bracket = brace = 0
    in_string = False
    quote = ""
    escaped = False

    for ch in text:
        if in_string:
            buf.append(ch)
            if escaped:
                escaped = False
            elif ch == "\\":
                escaped = True
            elif ch == quote:
                in_string = False
            continue

        if ch in ('"', "'"):
            in_string = True
            quote = ch
            buf.append(ch)
            continue

        if ch == "(":
            paren += 1
        elif ch == ")":
            paren -= 1
        elif ch == "[":
            bracket += 1
        elif ch == "]":
            bracket -= 1
        elif ch == "{":
            brace += 1
        elif ch == "}":
            brace -= 1

        if ch == "," and paren == 0 and bracket == 0 and brace == 0:
            fields.append("".join(buf).strip())
            buf = []
        else:
            buf.append(ch)

    if buf:
        fields.append("".join(buf).strip())

    return fields


def strip_atlas_codes(text: str) -> str:
    text = re.sub(r"=[a-zA-Z]+\d*=", "", text)
    text = re.sub(r"#(?:[a-zA-Z0-9_]+)#", "", text)
    text = text.strip()

    # AtlasLoot descriptions such as "=ds=#s1#, #a1#" become just ","
    # after removing formatting tokens. Treat punctuation-only leftovers as
    # empty rather than exporting meaningless description strings.
    if text and not re.search(r"\w", text, flags=re.UNICODE):
        return ""

    return text


def parse_loot_rows(table_body: str) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    row_re = re.compile(r"^\s*\{(.*)\}\s*;?\s*$")

    for line in table_body.splitlines():
        line = line.split("--", 1)[0].strip()
        if not line:
            continue

        match = row_re.match(line)
        if not match:
            continue

        fields = split_lua_fields(match.group(1))
        if len(fields) < 4:
            continue

        try:
            position = int(fields[0].strip())
        except ValueError:
            continue

        item_token = fields[1].strip()

        # Ignore headers, spells, submenu entries, etc.
        if not re.fullmatch(r"\d+", item_token):
            continue

        item_id = int(item_token)
        if item_id <= 0:
            continue

        name = strip_atlas_codes(first_quoted_string(fields[3]))
        description = (
            strip_atlas_codes(first_quoted_string(fields[4]))
            if len(fields) > 4 else ""
        )
        price = (
            strip_atlas_codes(first_quoted_string(fields[5]))
            if len(fields) > 5 else ""
        )
        drop_rate = (
            first_quoted_string(fields[6]).strip()
            if len(fields) > 6 else ""
        )

        items.append(
            {
                "position": position,
                "item_id": item_id,
                "atlas_name": name,
                "description": description or None,
                "price": price or None,
                "drop_rate": drop_rate or None,
            }
        )

    return items


def parse_instance_table_ids(
    dropdown_source: str,
    instance_key: str,
) -> list[str]:
    block = extract_named_table(dropdown_source, instance_key)
    if block is None:
        raise KeyError(
            f'Instance "{instance_key}" was not found in '
            f'{ATLAS_FILES["dropdown"]}'
        )

    ids: list[str] = []

    for line in block.splitlines():
        strings = re.findall(r'"((?:\\.|[^"\\])*)"', line)
        if strings:
            candidate = decode_lua_string(strings[-1])
            if candidate not in ids:
                ids.append(candidate)

    return ids


def parse_instance_zone_keys(dropdown_source: str) -> dict[str, str]:
    """
    Parse submenu declarations such as:
        { BabbleZone["Icecrown Citadel"], "IcecrownCitadel", "Submenu" },
    """
    result: dict[str, str] = {}
    pattern = re.compile(
        r'\{\s*BabbleZone\["((?:\\.|[^"\\])*)"\]\s*,\s*'
        r'"((?:\\.|[^"\\])*)"\s*,\s*"Submenu"\s*\}'
    )
    for zone_raw, key_raw in pattern.findall(dropdown_source):
        result[decode_lua_string(key_raw)] = decode_lua_string(zone_raw)
    return result


def parse_table_name_expressions(register_source: str) -> dict[str, str]:
    """
    Store the first expression from:
      AtlasLoot_TableNames["ICCLordMarrowgar"] =
        { BabbleBoss["Lord Marrowgar"], "AtlasLootWotLK" };
    """
    result: dict[str, str] = {}

    pattern = re.compile(
        r'AtlasLoot_TableNames\["([^"]+)"\]\s*=\s*\{\s*'
        r'(.+?)\s*,\s*"AtlasLoot[^"]*"\s*\}\s*;'
    )

    for line in register_source.splitlines():
        match = pattern.search(line)
        if match:
            result[match.group(1)] = match.group(2).strip()

    return result


def parse_table_name_overrides(source: str) -> dict[str, str]:
    """
    Parse localized explicit overrides such as:
      AtlasLoot_TableNames["KaraNamed"][1] = "Kellerbosse";

    Also accepts:
      AtlasLoot_TableNames["SomeID"] = { "Name", "..." };
    """
    result: dict[str, str] = {}

    direct = re.compile(
        r'AtlasLoot_TableNames\["([^"]+)"\]\[1\]\s*=\s*'
        r'"((?:\\.|[^"\\])*)"\s*;'
    )
    full = re.compile(
        r'AtlasLoot_TableNames\["([^"]+)"\]\s*=\s*\{\s*'
        r'"((?:\\.|[^"\\])*)"\s*,'
    )

    for table_id, value in direct.findall(source):
        result[table_id] = decode_lua_string(value)

    for table_id, value in full.findall(source):
        result[table_id] = decode_lua_string(value)

    return result


def parse_atlas_constants(source: str) -> dict[str, str]:
    """
    Parse:
      AL["Trash Mobs"] = "Trashmobs";
      AL["Something"] = true;   -> identity
    """
    result: dict[str, str] = {}

    quoted = re.compile(
        r'AL\["((?:\\.|[^"\\])*)"\]\s*=\s*'
        r'"((?:\\.|[^"\\])*)"\s*;?'
    )
    identity = re.compile(
        r'AL\["((?:\\.|[^"\\])*)"\]\s*=\s*true\s*;?'
    )

    for key, value in quoted.findall(source):
        result[decode_lua_string(key)] = decode_lua_string(value)

    for key in identity.findall(source):
        decoded = decode_lua_string(key)
        result.setdefault(decoded, decoded)

    return result


def extract_locale_section(source: str, locale: str) -> str:
    """
    Return the conditional section for a LibBabble locale.

    Works with the common:
      if GAME_LOCALE == "enUS" then
      elseif GAME_LOCALE == "deDE" then
      ...
    layout.
    """
    start_re = re.compile(
        r'(?:if|elseif)\s+GAME_LOCALE\s*==\s*["\']'
        + re.escape(locale)
        + r'["\']\s*then'
    )
    start = start_re.search(source)
    if not start:
        return ""

    next_re = re.compile(
        r'\n\s*(?:elseif\s+GAME_LOCALE\s*==|else\b|end\b)'
    )
    nxt = next_re.search(source, start.end())
    end = nxt.start() if nxt else len(source)
    return source[start.end():end]


def parse_libbabble_translations(
    source: str,
    locale: str,
) -> dict[str, str]:
    """
    Parse a LibBabble current-translation table for one locale.

    enUS normally uses SetCurrentTranslations(true), so an empty map is fine:
    callers fall back to the English key itself.
    """
    if locale == "enUS":
        return {}

    section = extract_locale_section(source, locale)
    if not section:
        return {}

    setter = re.search(r"lib:SetCurrentTranslations\s*\{", section)
    if not setter:
        return {}

    brace = section.find("{", setter.start())
    if brace < 0:
        return {}

    try:
        body = extract_braced_block(section, brace)
    except ValueError:
        return {}

    result: dict[str, str] = {}

    entry_re = re.compile(
        r'(?:\["((?:\\.|[^"\\])*)"\]|([A-Za-z_][A-Za-z0-9_]*))'
        r'\s*=\s*"((?:\\.|[^"\\])*)"'
    )

    for bracket_key, simple_key, raw_value in entry_re.findall(body):
        key = decode_lua_string(bracket_key or simple_key)
        value = decode_lua_string(raw_value)
        result[key] = value

    return result


NAME_TOKEN_RE = re.compile(
    r'BabbleBoss\["((?:\\.|[^"\\])*)"\]'
    r'|BabbleZone\["((?:\\.|[^"\\])*)"\]'
    r'|AL\["((?:\\.|[^"\\])*)"\]'
    r'|"((?:\\.|[^"\\])*)"'
)


def evaluate_table_name(
    expr: str,
    locale: str,
    boss_translations: dict[str, dict[str, str]],
    zone_translations: dict[str, dict[str, str]],
    atlas_constants: dict[str, dict[str, str]],
) -> str:
    """
    Evaluate the small subset of Lua concatenation used by loot-table names.
    """
    pieces: list[str] = []

    boss_map = boss_translations.get(locale, {})
    zone_map = zone_translations.get(locale, {})
    al_map = atlas_constants.get(locale, {})

    for match in NAME_TOKEN_RE.finditer(expr):
        boss_key, zone_key, al_key, literal = match.groups()

        if boss_key is not None:
            key = decode_lua_string(boss_key)
            pieces.append(boss_map.get(key, key))
        elif zone_key is not None:
            key = decode_lua_string(zone_key)
            pieces.append(zone_map.get(key, key))
        elif al_key is not None:
            key = decode_lua_string(al_key)
            pieces.append(al_map.get(key, key))
        elif literal is not None:
            pieces.append(decode_lua_string(literal))

    return "".join(pieces).strip()


def fallback_boss_name(table_id: str) -> str:
    name = re.sub(r"_(?:A|H)$", "", table_id)
    name = re.sub(r"25ManHEROIC$", "", name)
    name = re.sub(r"25Man$", "", name)
    name = re.sub(r"HEROIC$", "", name)
    return name


# ---------------------------------------------------------------------------
# Localized AtlasLoot names
# ---------------------------------------------------------------------------

def validate_locales(config: dict[str, Any]) -> list[str]:
    locales = config.get("locales", ["enUS"])
    if not isinstance(locales, list) or not locales:
        raise ValueError('"locales" must be a non-empty list')

    locales = [str(x) for x in locales]

    # English is always fetched because the stat parser is English-canonical.
    if "enUS" not in locales:
        locales.insert(0, "enUS")

    unsupported = [x for x in locales if x not in LOCALES]
    if unsupported:
        raise ValueError(
            "Unsupported locale(s): "
            + ", ".join(unsupported)
            + ". Supported: "
            + ", ".join(LOCALES)
        )

    # Preserve order but remove duplicates.
    return list(dict.fromkeys(locales))


def load_localization_data(
    config: dict[str, Any],
    locales: list[str],
    boss_source: str,
    zone_source: str,
) -> tuple[
    dict[str, dict[str, str]],
    dict[str, dict[str, str]],
    dict[str, dict[str, str]],
    dict[str, dict[str, str]],
]:
    boss_translations: dict[str, dict[str, str]] = {}
    zone_translations: dict[str, dict[str, str]] = {}
    atlas_constants: dict[str, dict[str, str]] = {}
    table_overrides: dict[str, dict[str, str]] = {}

    for locale in locales:
        boss_translations[locale] = parse_libbabble_translations(
            boss_source, locale
        )
        zone_translations[locale] = parse_libbabble_translations(
            zone_source, locale
        )

        atlas_suffix = LOCALES[locale]["atlas"]

        constants_path = f"AtlasLoot/Locales/constants.{atlas_suffix}.lua"
        constants_source = load_atlas_file(
            config,
            constants_path,
            optional=True,
        )
        atlas_constants[locale] = parse_atlas_constants(constants_source)

        if locale == "enUS":
            table_overrides[locale] = {}
        else:
            overrides_path = (
                f"AtlasLoot/TableRegister/loottables.{atlas_suffix}.lua"
            )
            overrides_source = load_atlas_file(
                config,
                overrides_path,
                optional=True,
            )
            table_overrides[locale] = parse_table_name_overrides(
                overrides_source
            )

    return (
        boss_translations,
        zone_translations,
        atlas_constants,
        table_overrides,
    )


def localized_table_names(
    table_ids: list[str],
    locales: list[str],
    expressions: dict[str, str],
    table_overrides: dict[str, dict[str, str]],
    boss_translations: dict[str, dict[str, str]],
    zone_translations: dict[str, dict[str, str]],
    atlas_constants: dict[str, dict[str, str]],
) -> dict[str, str]:
    names: dict[str, str] = {}

    # Find the best expression once; localized overrides still get first chance.
    expression = ""
    expression_table_id = ""
    for table_id in table_ids:
        if table_id in expressions:
            expression = expressions[table_id]
            expression_table_id = table_id
            break

    fallback = fallback_boss_name(
        expression_table_id or (table_ids[0] if table_ids else "Unknown")
    )

    for locale in locales:
        override_map = table_overrides.get(locale, {})

        override = None
        for table_id in table_ids:
            if table_id in override_map:
                override = override_map[table_id]
                break

        if override:
            names[locale] = override
            continue

        if expression:
            value = evaluate_table_name(
                expression,
                locale,
                boss_translations,
                zone_translations,
                atlas_constants,
            )
            names[locale] = value or fallback
        else:
            names[locale] = fallback

    return names


def localized_instance_names(
    instance_key: str,
    zone_key: str | None,
    locales: list[str],
    zone_translations: dict[str, dict[str, str]],
    config_entry: dict[str, Any] | None = None,
) -> dict[str, str]:
    names: dict[str, str] = {}
    override_names = (config_entry or {}).get("names", {})

    for locale in locales:
        if locale in override_names:
            names[locale] = str(override_names[locale])
            continue

        if zone_key:
            names[locale] = (
                zone_translations.get(locale, {}).get(zone_key, zone_key)
            )
        else:
            names[locale] = instance_key

    return names


# ---------------------------------------------------------------------------
# Difficulty / faction resolution
# ---------------------------------------------------------------------------

def infer_difficulty_label(
    base_id: str,
    suffix: str,
    all_tables: set[str],
) -> str:
    raid = any(
        (base_id + x) in all_tables
        or (base_id + x + "_A") in all_tables
        or (base_id + x + "_H") in all_tables
        for x in ("25Man", "25ManHEROIC")
    )

    if suffix == "":
        return "10_normal" if raid else "normal"
    if suffix == "HEROIC":
        return "10_heroic" if raid else "heroic"
    if suffix == "25Man":
        return "25_normal"
    if suffix == "25ManHEROIC":
        return "25_heroic"

    return suffix or "normal"


def requested_difficulty(label: str, config: dict[str, Any]) -> bool:
    requested = config.get("difficulties", ["all"])
    return "all" in requested or label in requested


def resolve_faction_tables(
    candidate: str,
    all_tables: set[str],
    faction_setting: str,
) -> list[tuple[str, str]]:
    a = candidate + "_A"
    h = candidate + "_H"
    has_a = a in all_tables
    has_h = h in all_tables

    if faction_setting == "A":
        if has_a:
            return [(a, "Alliance")]
        return [(candidate, "all")] if candidate in all_tables else []

    if faction_setting == "H":
        if has_h:
            return [(h, "Horde")]
        return [(candidate, "all")] if candidate in all_tables else []

    # both
    if has_a or has_h:
        out: list[tuple[str, str]] = []
        if has_a:
            out.append((a, "Alliance"))
        if has_h:
            out.append((h, "Horde"))
        return out

    return [(candidate, "all")] if candidate in all_tables else []


# ---------------------------------------------------------------------------
# Wowhead
# ---------------------------------------------------------------------------

def wowhead_lang(config: dict[str, Any], locale: str) -> str:
    overrides = config.get("wowhead_locale_slugs", {})
    if locale in overrides:
        return str(overrides[locale])
    return str(LOCALES[locale]["wowhead"])


def tooltip_url(config: dict[str, Any], item_id: int, locale: str) -> str:
    if locale == "enUS":
        template = config.get("wowhead_tooltip_api_en", TOOLTIP_API_EN)
        return str(template).format(item_id=item_id)

    lang = wowhead_lang(config, locale)
    template = config.get(
        "wowhead_tooltip_api_localized",
        TOOLTIP_API_LOCALIZED,
    )
    return str(template).format(item_id=item_id, lang=lang, locale=locale)


def fetch_wowhead_item(
    config: dict[str, Any],
    item_id: int,
    locale: str,
    cache_dir: Path,
    refresh: bool,
    delay_seconds: float,
) -> dict[str, Any]:
    locale_cache = cache_dir / locale
    locale_cache.mkdir(parents=True, exist_ok=True)
    cache_path = locale_cache / f"{item_id}.json"

    if cache_path.exists() and not refresh:
        try:
            return json.loads(cache_path.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            print(
                f"WARNING: corrupt cache for {locale} item {item_id}; "
                "refetching",
                file=sys.stderr,
            )

    url = tooltip_url(config, item_id, locale)
    raw = http_get_text(url)
    payload = json.loads(raw)

    cache_path.write_text(
        json.dumps(payload, ensure_ascii=False, indent=2),
        encoding="utf-8",
    )

    if delay_seconds > 0:
        time.sleep(delay_seconds)

    return payload


def tooltip_html_to_text(raw_html: str) -> str:
    text = raw_html
    text = re.sub(r"(?i)<br\s*/?>", "\n", text)
    text = re.sub(r"(?i)</(?:tr|div|p|li|table)>", "\n", text)
    text = re.sub(r"(?i)</(?:td|th)>", "\t", text)
    text = re.sub(r"<[^>]+>", "", text)
    text = html.unescape(text)
    text = text.replace("\xa0", " ")

    lines: list[str] = []
    for line in text.splitlines():
        line = re.sub(r"[ \t]+", " ", line).strip()
        if line:
            lines.append(line)

    return "\n".join(lines)


STAT_KEY_ALIASES = {
    "strength": "strength",
    "agility": "agility",
    "stamina": "stamina",
    "intellect": "intellect",
    "spirit": "spirit",
    "critical strike rating": "critical_strike_rating",
    "crit rating": "critical_strike_rating",
    "hit rating": "hit_rating",
    "haste rating": "haste_rating",
    "resilience rating": "resilience_rating",
    "defense rating": "defense_rating",
    "dodge rating": "dodge_rating",
    "parry rating": "parry_rating",
    "block rating": "block_rating",
    "expertise rating": "expertise_rating",
    "armor penetration rating": "armor_penetration_rating",
    "spell penetration": "spell_penetration",
    "attack power": "attack_power",
    "spell power": "spell_power",
}

SLOTS = (
    "Head", "Neck", "Shoulder", "Back", "Chest", "Shirt", "Tabard",
    "Wrist", "Hands", "Waist", "Legs", "Feet", "Finger", "Trinket",
    "Main Hand", "Off Hand", "One-Hand", "Two-Hand", "Ranged",
    "Thrown", "Relic",
)

ITEM_TYPES = (
    "Cloth", "Leather", "Mail", "Plate", "Shield",
    "Axe", "Axes", "Bow", "Bows", "Crossbow", "Crossbows",
    "Dagger", "Daggers", "Fist Weapon", "Gun", "Guns",
    "Mace", "Maces", "Polearm", "Polearms", "Staff", "Staves",
    "Sword", "Swords", "Wand", "Wands",
)


def add_stat(stats: dict[str, int], key: str, value: int) -> None:
    stats[key] = stats.get(key, 0) + value


def parse_socket_bonus(text: str) -> dict[str, Any]:
    result: dict[str, Any] = {"text": text}
    stats: dict[str, int] = {}

    for amount, name in re.findall(
        r"\+([\d,]+)\s+"
        r"(Strength|Agility|Stamina|Intellect|Spirit)",
        text,
        flags=re.I,
    ):
        add_stat(stats, name.lower(), int(amount.replace(",", "")))

    generic = re.search(
        r"\+([\d,]+)\s+"
        r"(Critical Strike Rating|Hit Rating|Haste Rating|"
        r"Resilience Rating|Defense Rating|Dodge Rating|Parry Rating|"
        r"Block Rating|Expertise Rating|Armor Penetration Rating|"
        r"Spell Penetration|Attack Power|Spell Power)",
        text,
        flags=re.I,
    )
    if generic:
        key = STAT_KEY_ALIASES.get(generic.group(2).lower())
        if key:
            add_stat(stats, key, int(generic.group(1).replace(",", "")))

    if stats:
        result["stats"] = stats

    return result


def parse_tooltip_payload(
    item_id: int,
    payload: dict[str, Any],
    atlas_name: str = "",
) -> dict[str, Any]:
    """
    Parse CANONICAL DATA FROM ENGLISH TOOLTIP ONLY.
    """
    tooltip_html = str(payload.get("tooltip") or "")
    text = tooltip_html_to_text(tooltip_html)

    item: dict[str, Any] = {
        "id": item_id,
        "name": payload.get("name") or atlas_name or None,
        "icon": payload.get("icon"),
    }

    for key in ("quality", "level", "itemLevel", "displayId"):
        if key in payload and payload[key] is not None:
            item[key] = payload[key]

    def first_int(pattern: str) -> int | None:
        match = re.search(pattern, text, flags=re.I | re.M)
        if not match:
            return None
        return int(match.group(1).replace(",", ""))

    item_level = first_int(r"\bItem Level\s+([\d,]+)\b")
    if item_level is not None:
        item["item_level"] = item_level

    required_level = first_int(r"\bRequires Level\s+([\d,]+)\b")
    if required_level is not None:
        item["required_level"] = required_level

    binding_map = {
        "Binds when picked up": "bind_on_pickup",
        "Binds when equipped": "bind_on_equip",
        "Binds to account": "bind_to_account",
        "Binds to Battle.net account": "bind_to_bnet_account",
    }
    for phrase, value in binding_map.items():
        if phrase.lower() in text.lower():
            item["binding"] = value
            break

    lines = text.splitlines()

    for slot in SLOTS:
        if any(line.casefold() == slot.casefold() for line in lines):
            item["slot"] = slot
            break

    for item_type in ITEM_TYPES:
        if any(line.casefold() == item_type.casefold() for line in lines):
            item["type"] = item_type
            break

    armor = first_int(r"(?m)^([\d,]+)\s+Armor$")
    if armor is not None:
        item["armor"] = armor

    damage = re.search(
        r"([\d,]+)\s*-\s*([\d,]+)\s+Damage",
        text,
        flags=re.I,
    )
    speed = re.search(r"\bSpeed\s+([\d.]+)", text, flags=re.I)
    dps = re.search(
        r"\(([\d.]+)\s+damage per second\)",
        text,
        flags=re.I,
    )

    if damage or speed or dps:
        weapon: dict[str, Any] = {}
        if damage:
            weapon["min_damage"] = int(damage.group(1).replace(",", ""))
            weapon["max_damage"] = int(damage.group(2).replace(",", ""))
        if speed:
            weapon["speed"] = float(speed.group(1))
        if dps:
            weapon["dps"] = float(dps.group(1))
        item["weapon"] = weapon

    stats: dict[str, int] = {}

    for amount, name in re.findall(
        r"\+([\d,]+)\s+"
        r"(Strength|Agility|Stamina|Intellect|Spirit)\b",
        text,
        flags=re.I,
    ):
        add_stat(stats, name.lower(), int(amount.replace(",", "")))

    for amount, school in re.findall(
        r"\+([\d,]+)\s+"
        r"(Arcane|Fire|Frost|Nature|Shadow)\s+Resistance\b",
        text,
        flags=re.I,
    ):
        add_stat(
            stats,
            school.lower() + "_resistance",
            int(amount.replace(",", "")),
        )

    for label, amount in re.findall(
        r"(?:Improves|Increases)(?: your)?\s+"
        r"([A-Za-z ]+?)\s+by\s+([\d,]+)",
        text,
        flags=re.I,
    ):
        normalized = re.sub(r"\s+", " ", label.strip().lower())
        key = STAT_KEY_ALIASES.get(normalized)
        if key:
            add_stat(stats, key, int(amount.replace(",", "")))

    shield_block = re.search(
        r"Increases the block value of your shield by\s+([\d,]+)",
        text,
        flags=re.I,
    )
    if shield_block:
        add_stat(
            stats,
            "shield_block_value",
            int(shield_block.group(1).replace(",", "")),
        )

    mp5 = re.search(
        r"Restores\s+([\d,]+)\s+mana per 5 sec",
        text,
        flags=re.I,
    )
    if mp5:
        add_stat(stats, "mana_per_5", int(mp5.group(1).replace(",", "")))

    if stats:
        item["stats"] = stats

    sockets = [
        color.lower()
        for color in re.findall(
            r"\b(Red|Blue|Yellow|Meta|Prismatic)\s+Socket\b",
            text,
            flags=re.I,
        )
    ]
    if sockets:
        item["sockets"] = sockets

    socket_bonus_match = re.search(
        r"Socket Bonus:\s*([^\n]+)",
        text,
        flags=re.I,
    )
    if socket_bonus_match:
        item["socket_bonus"] = parse_socket_bonus(
            socket_bonus_match.group(1).strip()
        )

    effects = [
        line for line in lines
        if re.match(
            r"^(Equip:|Use:|Chance on hit:|Chance on melee hit:|"
            r"Chance on spell hit:)",
            line,
            flags=re.I,
        )
    ]
    if effects:
        item["effects"] = effects

    requirements = [
        line for line in lines
        if line.startswith("Requires ")
        and not re.match(r"^Requires Level\s+\d+", line)
    ]
    if requirements:
        item["requirements"] = requirements

    if any(line == "Unique" or line.startswith("Unique-") for line in lines):
        item["unique"] = True

    return item


def localized_item_view(
    payload: dict[str, Any],
    *,
    include_tooltips: bool,
) -> dict[str, Any]:
    view: dict[str, Any] = {
        "name": payload.get("name"),
    }

    if payload.get("icon"):
        view["icon"] = payload["icon"]

    if include_tooltips:
        tooltip_html = str(payload.get("tooltip") or "")
        view["tooltip_html"] = tooltip_html
        view["tooltip_text"] = tooltip_html_to_text(tooltip_html)

    return view


# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

def build_database(
    config: dict[str, Any],
    refresh: bool = False,
) -> dict[str, Any]:
    locales = validate_locales(config)
    default_locale = str(config.get("default_locale", "enUS"))
    if default_locale not in locales:
        raise ValueError(
            f'default_locale "{default_locale}" is not in "locales"'
        )

    dropdown_source = load_atlas_file(config, ATLAS_FILES["dropdown"])
    register_source = load_atlas_file(config, ATLAS_FILES["register_en"])
    loot_source = load_atlas_file(config, ATLAS_FILES["loot"])
    boss_source = load_atlas_file(config, ATLAS_FILES["boss_babble"])
    zone_source = load_atlas_file(config, ATLAS_FILES["zone_babble"])

    loot_tables = parse_all_loot_tables(loot_source)
    all_table_ids = set(loot_tables)
    table_expressions = parse_table_name_expressions(register_source)
    instance_zone_keys = parse_instance_zone_keys(dropdown_source)

    (
        boss_translations,
        zone_translations,
        atlas_constants,
        table_overrides,
    ) = load_localization_data(
        config,
        locales,
        boss_source,
        zone_source,
    )

    instances_cfg = config.get("instances")
    if not instances_cfg:
        raise ValueError('config.json must contain a non-empty "instances" list')

    include_trash = bool(config.get("include_trash", True))
    faction_setting = str(config.get("faction", "both")).upper()
    if faction_setting not in {"A", "H", "BOTH"}:
        raise ValueError('"faction" must be "A", "H", or "both"')

    cache_dir = Path(config.get("cache_dir", ".cache/wowhead"))
    delay_seconds = float(config.get("request_delay_seconds", 0.20))
    include_localized_tooltips = bool(
        config.get("include_localized_tooltips", True)
    )
    include_raw_payload = bool(config.get("include_raw_payload", False))
    localized_fetch_policy = str(
        config.get("localized_fetch_policy", "best_effort")
    )
    if localized_fetch_policy not in {"best_effort", "strict"}:
        raise ValueError(
            '"localized_fetch_policy" must be "best_effort" or "strict"'
        )

    output_instances: dict[str, Any] = {}
    needed_item_ids: set[int] = set()
    atlas_name_by_id: dict[int, str] = {}
    skipped_refs: list[dict[str, str]] = []

    for instance_entry in instances_cfg:
        config_obj: dict[str, Any] | None

        if isinstance(instance_entry, str):
            instance_key = instance_entry
            config_obj = None
        elif isinstance(instance_entry, dict):
            instance_key = str(instance_entry["key"])
            config_obj = instance_entry
        else:
            raise ValueError(
                'Each "instances" entry must be a string or {"key": "..."}'
            )

        zone_key = instance_zone_keys.get(instance_key)
        instance_names = localized_instance_names(
            instance_key,
            zone_key,
            locales,
            zone_translations,
            config_obj,
        )

        base_ids = parse_instance_table_ids(
            dropdown_source,
            instance_key,
        )

        instance_obj: dict[str, Any] = {
            "id": instance_key,
            "names": instance_names,
            "bosses": {},
        }

        for base_id in base_ids:
            possible: list[str] = []
            for suffix in DIFFICULTY_SUFFIXES:
                candidate = base_id + suffix
                possible.extend(
                    [candidate, candidate + "_A", candidate + "_H"]
                )

            if not any(x in all_table_ids for x in possible):
                skipped_refs.append(
                    {"instance": instance_key, "table_ref": base_id}
                )
                continue

            # Stable boss/source ID: base table ID, not translated name.
            boss_obj = instance_obj["bosses"].setdefault(
                base_id,
                {
                    "id": base_id,
                    "names": {},
                    "loot": defaultdict(list),
                },
            )

            for suffix in DIFFICULTY_SUFFIXES:
                difficulty = infer_difficulty_label(
                    base_id,
                    suffix,
                    all_table_ids,
                )
                if not requested_difficulty(difficulty, config):
                    continue

                candidate = base_id + suffix
                resolved = resolve_faction_tables(
                    candidate,
                    all_table_ids,
                    faction_setting,
                )

                for actual_table_id, faction in resolved:
                    names = localized_table_names(
                        [actual_table_id, candidate, base_id],
                        locales,
                        table_expressions,
                        table_overrides,
                        boss_translations,
                        zone_translations,
                        atlas_constants,
                    )

                    for locale, value in names.items():
                        boss_obj["names"].setdefault(locale, value)

                    # Trash filtering uses the English name when available,
                    # but also falls back to stable ID.
                    english_name = boss_obj["names"].get(
                        "enUS",
                        fallback_boss_name(base_id),
                    )
                    if (
                        not include_trash
                        and (
                            "trash" in english_name.lower()
                            or "trash" in base_id.lower()
                        )
                    ):
                        continue

                    rows = parse_loot_rows(loot_tables[actual_table_id])
                    if not rows:
                        continue

                    for row in rows:
                        item_id = row["item_id"]
                        needed_item_ids.add(item_id)

                        if (
                            row.get("atlas_name")
                            and item_id not in atlas_name_by_id
                        ):
                            atlas_name_by_id[item_id] = row["atlas_name"]

                        boss_obj["loot"][difficulty].append(
                            {
                                "item_id": item_id,
                                "drop_rate": row.get("drop_rate"),
                                "price": row.get("price"),
                                "description": row.get("description"),
                                "table_id": actual_table_id,
                                "faction": faction,
                                "position": row.get("position"),
                            }
                        )

        # Drop empty boss/source objects and convert defaultdicts.
        cleaned_bosses: dict[str, Any] = {}
        for boss_id, boss_obj in instance_obj["bosses"].items():
            boss_obj["loot"] = dict(boss_obj["loot"])
            if boss_obj["loot"]:
                cleaned_bosses[boss_id] = boss_obj

        instance_obj["bosses"] = cleaned_bosses
        output_instances[instance_key] = instance_obj

    print(
        f"Found {len(needed_item_ids)} unique item IDs across "
        f"{len(output_instances)} configured instance(s)."
    )
    print("Locales:", ", ".join(locales))

    items_out: dict[str, Any] = {}
    failures: list[dict[str, Any]] = []
    locale_failures: list[dict[str, Any]] = []

    total_fetches = len(needed_item_ids) * len(locales)
    fetch_counter = 0

    for item_index, item_id in enumerate(sorted(needed_item_ids), start=1):
        atlas_name = atlas_name_by_id.get(item_id, "")

        # English is mandatory because it is the canonical stat source.
        fetch_counter += 1
        print(
            f"[item {item_index}/{len(needed_item_ids)} | "
            f"fetch {fetch_counter}/{total_fetches}] "
            f"{item_id} enUS",
            end=" ... ",
        )

        try:
            english_payload = fetch_wowhead_item(
                config=config,
                item_id=item_id,
                locale="enUS",
                cache_dir=cache_dir,
                refresh=refresh,
                delay_seconds=delay_seconds,
            )

            item = parse_tooltip_payload(
                item_id,
                english_payload,
                atlas_name,
            )
            item["locales"] = {
                "enUS": localized_item_view(
                    english_payload,
                    include_tooltips=include_localized_tooltips,
                )
            }

            if include_raw_payload:
                item["locales"]["enUS"]["raw"] = english_payload

            print("ok")

        except Exception as exc:
            print(f"FAILED: {exc}")
            failures.append(
                {
                    "item_id": item_id,
                    "locale": "enUS",
                    "error": str(exc),
                }
            )
            items_out[str(item_id)] = {
                "id": item_id,
                "name": atlas_name or None,
                "locales": {
                    "enUS": {
                        "name": atlas_name or None,
                        "fetch_error": str(exc),
                    }
                },
                "fetch_error": str(exc),
            }
            # Without English we cannot safely parse canonical item data.
            continue

        # Fetch only the extra languages; enUS was already done.
        for locale in locales:
            if locale == "enUS":
                continue

            fetch_counter += 1
            print(
                f"[item {item_index}/{len(needed_item_ids)} | "
                f"fetch {fetch_counter}/{total_fetches}] "
                f"{item_id} {locale}",
                end=" ... ",
            )

            try:
                payload = fetch_wowhead_item(
                    config=config,
                    item_id=item_id,
                    locale=locale,
                    cache_dir=cache_dir,
                    refresh=refresh,
                    delay_seconds=delay_seconds,
                )

                item["locales"][locale] = localized_item_view(
                    payload,
                    include_tooltips=include_localized_tooltips,
                )

                if include_raw_payload:
                    item["locales"][locale]["raw"] = payload

                print("ok")

            except Exception as exc:
                print(f"FAILED: {exc}")

                locale_failures.append(
                    {
                        "item_id": item_id,
                        "locale": locale,
                        "error": str(exc),
                    }
                )

                if localized_fetch_policy == "strict":
                    raise

                # Best-effort mode keeps the database usable and makes the
                # fallback explicit.
                item["locales"][locale] = {
                    "name": item["locales"]["enUS"].get("name"),
                    "fallback_locale": "enUS",
                    "fetch_error": str(exc),
                }

        items_out[str(item_id)] = item

    db: dict[str, Any] = {
        "meta": {
            "schema_version": 2,
            "generated_at": datetime.now(timezone.utc).isoformat(),
            "atlasloot_repo": config.get("atlasloot_repo", DEFAULT_REPO),
            "atlasloot_branch": config.get(
                "atlasloot_branch",
                DEFAULT_BRANCH,
            ),
            "locales": locales,
            "default_locale": default_locale,
            "canonical_stat_locale": "enUS",
            "configured_instances": instances_cfg,
            "unique_items": len(needed_item_ids),
            "failed_english_item_fetches": len(failures),
            "failed_localized_item_fetches": len(locale_failures),
        },
        "items": items_out,
        "instances": output_instances,
    }

    if failures:
        db["meta"]["failures"] = failures

    if locale_failures:
        db["meta"]["locale_failures"] = locale_failures

    if skipped_refs:
        db["meta"]["skipped_non_wotlk_table_refs"] = skipped_refs

    return db


def main() -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Build multilingual WotLK boss/item/stat JSON from "
            "AtlasLoot + Wowhead"
        )
    )
    parser.add_argument(
        "config",
        nargs="?",
        default="config.json",
        help="Config JSON path (default: config.json)",
    )
    parser.add_argument(
        "--refresh",
        action="store_true",
        help="Ignore cached Wowhead responses and refetch",
    )
    args = parser.parse_args()

    config_path = Path(args.config)
    if not config_path.exists():
        print(f"Config not found: {config_path}", file=sys.stderr)
        return 2

    config = json.loads(config_path.read_text(encoding="utf-8"))

    try:
        db = build_database(config, refresh=args.refresh)
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1

    output_path = Path(config.get("output", "loot.localized.json"))
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(
        json.dumps(db, ensure_ascii=False, indent=2),
        encoding="utf-8",
    )

    print(f"\nWrote: {output_path}")
    print(f"Items: {len(db['items'])}")
    print(f"Instances: {len(db['instances'])}")
    print(
        "Localized fetch failures: "
        f"{db['meta']['failed_localized_item_fetches']}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
