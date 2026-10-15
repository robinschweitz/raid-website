import argparse
import http.cookiejar
import json
import logging
import os
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime

# --------------------------------------------------
# Configuration
# --------------------------------------------------

skip_ids = {5, 8, 13, 14, 16, 25, 27}
base = "https://bratwurst-des-schicksals.de"

REQUEST_TIMEOUT = 30
REQUEST_DELAY = 2
LOG_FILE = "character_linking.log"

USERNAME = os.getenv("API_USERNAME", "admin")
PASSWORD = "xd92x5PhiXSNF7jpo0MhGoVF11nScacG"

# --------------------------------------------------
# Logging setup
# --------------------------------------------------

parser = argparse.ArgumentParser()
parser.add_argument(
    "--debug",
    action="store_true",
    help="Enable detailed debug logging",
)
args = parser.parse_args()

log_level = logging.DEBUG if args.debug else logging.INFO

logging.basicConfig(
    level=log_level,
    format="%(asctime)s | %(levelname)-8s | %(message)s",
    handlers=[
        logging.StreamHandler(sys.stdout),
        logging.FileHandler(LOG_FILE, encoding="utf-8"),
    ],
)

logger = logging.getLogger("character_linker")

logger.info("=" * 70)
logger.info("Character linking process started")
logger.info("=" * 70)

logger.info("Base URL: %s", base)
logger.info("Username: %s", USERNAME)
logger.info("Request timeout: %ss", REQUEST_TIMEOUT)
logger.info("Delay between requests: %ss", REQUEST_DELAY)
logger.info("Excluded character IDs: %s", sorted(skip_ids))
logger.info("Debug logging: %s", args.debug)
logger.info("Log file: %s", os.path.abspath(LOG_FILE))

if not PASSWORD:
    logger.critical("API_PASSWORD environment variable is not set")
    sys.exit(1)

# --------------------------------------------------
# HTTP session
# --------------------------------------------------

jar = http.cookiejar.CookieJar()

opener = urllib.request.build_opener(
    urllib.request.HTTPCookieProcessor(jar)
)

request_counter = 0


def request(method, path, payload=None):
    global request_counter

    request_counter += 1
    request_id = request_counter

    url = base + path

    body = (
        None
        if payload is None
        else json.dumps(payload).encode("utf-8")
    )

    headers = (
        {}
        if body is None
        else {"Content-Type": "application/json"}
    )

    # Never log authentication credentials.
    if path == "/api/v1/auth/login":
        safe_payload = {"username": USERNAME, "password": "***"}
    else:
        safe_payload = payload

    logger.info(
        "[HTTP #%s] --> %s %s",
        request_id,
        method,
        path,
    )

    if safe_payload is not None:
        logger.debug(
            "[HTTP #%s] Request payload: %s",
            request_id,
            json.dumps(safe_payload, ensure_ascii=False),
        )

    req = urllib.request.Request(
        url,
        data=body,
        headers=headers,
        method=method,
    )

    started = time.monotonic()

    try:
        with opener.open(req, timeout=REQUEST_TIMEOUT) as response:
            status = response.status
            raw = response.read()
            elapsed = time.monotonic() - started

            logger.info(
                "[HTTP #%s] <-- HTTP %s | %.2fs | %s bytes",
                request_id,
                status,
                elapsed,
                len(raw),
            )

            try:
                data = json.loads(raw)
            except json.JSONDecodeError:
                logger.error(
                    "[HTTP #%s] Failed to decode JSON response",
                    request_id,
                )
                logger.debug(
                    "[HTTP #%s] Raw response: %s",
                    request_id,
                    raw.decode("utf-8", errors="replace"),
                )
                raise

            if path == "/api/v1/auth/login":
                logger.debug(
                    "[HTTP #%s] Login response received (body hidden)",
                    request_id,
                )
            else:
                logger.debug(
                    "[HTTP #%s] Response body: %s",
                    request_id,
                    json.dumps(data, ensure_ascii=False),
                )

            return status, data

    except urllib.error.HTTPError as error:
        elapsed = time.monotonic() - started
        raw = error.read()

        logger.error(
            "[HTTP #%s] HTTP ERROR %s | %.2fs | %s",
            request_id,
            error.code,
            elapsed,
            error.reason,
        )

        try:
            data = json.loads(raw)
        except json.JSONDecodeError:
            logger.error(
                "[HTTP #%s] Error response is not valid JSON",
                request_id,
            )
            data = {
                "error": {
                    "code": "invalid_json",
                    "message": raw.decode("utf-8", errors="replace"),
                }
            }

        logger.debug(
            "[HTTP #%s] Error response: %s",
            request_id,
            json.dumps(data, ensure_ascii=False),
        )

        return error.code, data

    except urllib.error.URLError as error:
        elapsed = time.monotonic() - started

        logger.exception(
            "[HTTP #%s] Connection error after %.2fs: %s",
            request_id,
            elapsed,
            error.reason,
        )
        raise

    except TimeoutError:
        elapsed = time.monotonic() - started

        logger.exception(
            "[HTTP #%s] Request timed out after %.2fs",
            request_id,
            elapsed,
        )
        raise


def sleep_with_log(seconds, reason):
    logger.info(
        "[WAIT] Sleeping %s seconds | Reason: %s",
        seconds,
        reason,
    )
    time.sleep(seconds)


# --------------------------------------------------
# Authentication
# --------------------------------------------------

script_started = time.monotonic()

logger.info("")
logger.info("=" * 70)
logger.info("STEP 1: AUTHENTICATION")
logger.info("=" * 70)

logger.info("Attempting login as '%s'", USERNAME)

status, response = request(
    "POST",
    "/api/v1/auth/login",
    {
        "username": USERNAME,
        "password": PASSWORD,
    },
)

logger.info("Login HTTP status: %s", status)

if status != 200:
    logger.critical("Authentication failed")
    logger.critical(
        "Response: %s",
        json.dumps(response, ensure_ascii=False),
    )
    sys.exit(1)

if response.get("data", {}).get("mfa_required"):
    logger.critical("Login requires MFA")
    logger.critical("Cannot continue automatically")
    sys.exit(1)

logger.info("Authentication successful")
logger.info("Session cookies received: %s", len(jar))

for cookie in jar:
    logger.debug(
        "Session cookie: name=%s domain=%s secure=%s",
        cookie.name,
        cookie.domain,
        cookie.secure,
    )

# --------------------------------------------------
# Retrieve characters
# --------------------------------------------------

logger.info("")
logger.info("=" * 70)
logger.info("STEP 2: FETCH CHARACTERS")
logger.info("=" * 70)

status, response = request(
    "GET",
    "/api/v1/characters",
)

if status != 200:
    logger.critical(
        "Failed to retrieve characters (HTTP %s)",
        status,
    )
    logger.critical(
        "Response: %s",
        json.dumps(response, ensure_ascii=False),
    )
    sys.exit(1)

characters = response["data"]

logger.info("Retrieved %s characters", len(characters))

# --------------------------------------------------
# Filter characters
# --------------------------------------------------

logger.info("")
logger.info("=" * 70)
logger.info("STEP 3: FILTER CHARACTERS")
logger.info("=" * 70)

unlinked = []

already_linked = []
excluded = []

for character in characters:
    character_id = character["id"]
    name = character.get("name", "<unknown>")

    external_source = character.get("external_source")
    external_profile_id = character.get("external_profile_id")

    logger.debug(
        "[FILTER] ID=%s | Name=%s | External source=%s | External profile=%s",
        character_id,
        name,
        external_source,
        external_profile_id,
    )

    if character_id in skip_ids:
        excluded.append(character)

        logger.info(
            "[FILTER] SKIP ID=%s | %s | Explicitly excluded",
            character_id,
            name,
        )
        continue

    if external_source or external_profile_id:
        already_linked.append(character)

        logger.info(
            "[FILTER] SKIP ID=%s | %s | Already linked",
            character_id,
            name,
        )
        continue

    unlinked.append(character)

    logger.info(
        "[FILTER] ELIGIBLE ID=%s | %s",
        character_id,
        name,
    )

logger.info("")
logger.info("Filtering summary:")
logger.info("  Total characters: %s", len(characters))
logger.info("  Already linked:    %s", len(already_linked))
logger.info("  Explicitly skipped:%s", len(excluded))
logger.info("  Eligible:          %s", len(unlinked))

# --------------------------------------------------
# Report
# --------------------------------------------------

report = {
    "eligible": len(unlinked),
    "deferred": sorted(skip_ids),
    "linked": [],
    "skipped": [],
    "stopped": False,
}

# --------------------------------------------------
# Linking process
# --------------------------------------------------

logger.info("")
logger.info("=" * 70)
logger.info("STEP 4: START LINKING")
logger.info("=" * 70)

logger.info(
    "Starting processing of %s eligible characters",
    len(unlinked),
)

sleep_with_log(
    REQUEST_DELAY,
    "Initial cooldown before starting linking",
)

for index, character in enumerate(unlinked):
    character_id = character["id"]
    character_name = character["name"]

    position = index + 1
    total = len(unlinked)

    logger.info("")
    logger.info("-" * 70)

    logger.info(
        "[%s/%s] Processing character ID=%s | Name=%s",
        position,
        total,
        character_id,
        character_name,
    )

    logger.info(
        "[%s/%s] Progress: %.1f%%",
        position,
        total,
        (position / total) * 100,
    )

    logger.info("-" * 70)

    character_started = time.monotonic()

    status, response = request(
        "POST",
        f"/api/v1/characters/{character_id}/retry-link",
    )

    elapsed = time.monotonic() - character_started

    record = {
        "id": character_id,
        "name": character_name,
        "status": status,
    }

    logger.info(
        "[%s/%s] Retry-link response: HTTP %s (%.2fs)",
        position,
        total,
        status,
        elapsed,
    )

    if status == 200:
        logger.info(
            "[SUCCESS] Character ID=%s | %s linked successfully",
            character_id,
            character_name,
        )

        report["linked"].append(record)

    else:
        error = response.get("error", {})

        record["code"] = error.get("code", "unknown")

        logger.warning(
            "[FAILED] Character ID=%s | %s",
            character_id,
            character_name,
        )

        logger.warning(
            "[FAILED] HTTP status: %s",
            status,
        )

        logger.warning(
            "[FAILED] Error code: %s",
            record["code"],
        )

        logger.debug(
            "[FAILED] Complete API response: %s",
            json.dumps(response, ensure_ascii=False),
        )

        if (
            status == 502
            and record["code"] == "rising_gods_unavailable"
        ):
            logger.warning(
                "[UPSTREAM] Rising Gods appears unavailable"
            )

            logger.info(
                "[UPSTREAM] Running control query to determine "
                "whether failure is global or profile-specific"
            )

            sleep_with_log(
                REQUEST_DELAY,
                "Cooldown before Rising Gods control request",
            )

            control_status, control_response = request(
                "GET",
                "/api/v1/external/rising-gods/characters"
                "?q=eu.rising-gods.abadi",
            )

            record["control_status"] = control_status

            logger.info(
                "[CONTROL] HTTP status: %s",
                control_status,
            )

            logger.debug(
                "[CONTROL] Full response: %s",
                json.dumps(
                    control_response,
                    ensure_ascii=False,
                ),
            )

            if control_status != 200:
                logger.critical(
                    "[CONTROL] Control query failed"
                )

                logger.critical(
                    "[CONTROL] Rising Gods may be globally unavailable"
                )

                logger.critical(
                    "[STOP] Aborting remaining character attempts"
                )

                report["skipped"].append(record)
                report["stopped"] = True

                break

            logger.info(
                "[CONTROL] Control query succeeded"
            )

            logger.warning(
                "[CONTROL] Failure appears profile-specific"
            )

            record["code"] = (
                "profile_specific_upstream_403"
            )

        report["skipped"].append(record)

    # --------------------------------------------------
    # Progress summary
    # --------------------------------------------------

    logger.info(
        "[PROGRESS] Linked=%s | Failed=%s | Remaining=%s",
        len(report["linked"]),
        len(report["skipped"]),
        total - position,
    )

    if index + 1 < len(unlinked):
        sleep_with_log(
            REQUEST_DELAY,
            "Rate limiting between character link attempts",
        )

# --------------------------------------------------
# Final summary
# --------------------------------------------------

total_elapsed = time.monotonic() - script_started

logger.info("")
logger.info("=" * 70)
logger.info("FINAL EXECUTION SUMMARY")
logger.info("=" * 70)

logger.info("Eligible characters: %s", report["eligible"])
logger.info("Successfully linked: %s", len(report["linked"]))
logger.info("Failed / skipped:    %s", len(report["skipped"]))
logger.info("Stopped early:       %s", report["stopped"])
logger.info("Total HTTP requests: %s", request_counter)
logger.info("Execution time:      %.2f seconds", total_elapsed)

if report["linked"]:
    logger.info("")
    logger.info("Successfully linked characters:")

    for record in report["linked"]:
        logger.info(
            "  [OK] ID=%s | %s",
            record["id"],
            record["name"],
        )

if report["skipped"]:
    logger.info("")
    logger.info("Failed / skipped characters:")

    for record in report["skipped"]:
        logger.warning(
            "  [FAIL] ID=%s | %s | HTTP=%s | Code=%s",
            record["id"],
            record["name"],
            record["status"],
            record.get("code", "unknown"),
        )

logger.info("")
logger.info("Complete JSON report:")

print(
    json.dumps(
        report,
        ensure_ascii=False,
        indent=2,
    )
)

logger.info("=" * 70)
logger.info("Character linking process finished")
logger.info("=" * 70)