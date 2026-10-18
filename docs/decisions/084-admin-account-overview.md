# ADR 084: Admin overview of who has claimed which player

Status: Accepted.

## Context

Players were imported from the old list before anyone had a login. When someone registers with a
code, the login is attached to the player who already owns that character (matched by Rising-Gods
profile, then by character name), so the player's name can differ from the login and the character
(for example player "Mira", character "Mirabelle", login "mira"). Admins could not see which players
had been claimed, by whom, or who still had no login: the player list showed only name and status,
and a redeemed code showed only "verwendet von @login" inside the request's details.

## Decision

Read-only; no data, registration or permission rule changes.

- **`GET /api/v1/player-accounts` (ADMIN)** lists every player with role, login name, characters and
  an account origin:
  - `REGISTERED`: a new-account registration code was redeemed for this player. The first such
    redemption gives `registered_at`, `registered_character` and `request_id`.
  - `ADMIN`: the player can log in but no code was redeemed (bootstrap admin, or credentials set by
    an Admin).
  - `NONE`: no login yet.
  Origins are derived from `registration_codes.used_by_player_id`; nothing new is stored.
- **Registration requests** in the Admin list carry `linked_player` (player ID, name, login, date)
  once a code was redeemed, for new accounts and character claims alike.
- **Players page for Admins:** columns Charaktere, Konto and Rolle, a summary ("9 von 11 Spielern
  haben ein Konto"), a Konto filter (Alle, Mit Konto, Ohne Konto), and search across player,
  character and login names. Other roles see the page as before and do not call the endpoint.
- **Registrierung → Verlauf:** a "Verknüpfter Spieler" column links to the player, with login and
  date. Request statuses are shown in German (Offen, Code erstellt, Abgelehnt, Eingelöst).
- The public `GET /api/v1/players` stays unchanged and never exposes login names (ADR 067).

## Consequences

- Admins can see at a glance who still has to register, and which person ended up on which imported
  player, which makes wrong matches visible early (they are fixed with the existing account merge).
- Logins set directly by an Admin are reported as `ADMIN` even if the person later used a code for a
  different player; the code then shows up on that other player.
