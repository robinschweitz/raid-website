# Requirements — raid-website

This file owns WHAT the system must do. It describes the behaviour of version 1.0.27
(2026-10-22), derived from `AGENTS.md`, `docs/decisions/*.md`, and the code. The text in
parentheses after a requirement names its source decision. The ADRs keep the reasons; this
file keeps the current rule. If code and an ADR disagree, the item is in `## Open Questions`.
A requirement marked "Owner 2026-10-22" is a decision from the owner's question and review pass. It is not
implemented yet when it differs from the code.

## Required Requirement Inputs

- Project purpose: Manage the raids of the World of Warcraft (WotLK 3.3.5a) guild "Bratwurst des Schicksals" on the Rising-Gods realm: raid planning and signups, rosters, loot, Enhance Points (EP), item locks, armor tokens, wishlists, polls, and the Shadowmourne questline.
- Primary users / actors: Visitor (no session); GUEST; MEMBER; RAIDASSISTENT; RAIDLEADER; ADMIN; the in-game addons RaidLedger and RaidWishlist (through copy and paste); the Rising-Gods profiler (external system).
- Core workflows: Register with a code; sign up for a raid or report an absence; build the raid roster; record loot during or after the raid; archive the raid (award EP, expire locks, schedule the successor); correct history with an audit trail; maintain a wishlist and export it to RaidWishlist; exchange invite, roster, and loot data with RaidLedger.
- Business objects / data entities: Player (account); Character (Charakter); Guild rank; Raid group; Raid series; Raid; Signup (Anmeldung); Absence (Absage / Unsicher); Attendance row (Aufstellung, Ersatzbank); Loot award; Unclaimed loot; EP transaction; Item lock; Wishlist item; Poll and vote; Registration request and code; Shadowmourne entry and raid task; Recruitment template (Gesuchtext); Boss order; Boss kill; Audit entry; Loot catalog (bundled, read-only).
- External integrations: Rising-Gods profiler `db.rising-gods.de` (character lookup), optionally through a guild-owned Cloudflare Worker; RaidLedger and RaidWishlist addons (text formats RS1, RS2, WL2; bundled downloads).
- Authentication / roles: Local accounts with password; optional TOTP, recovery codes, and WebAuthn security keys; cookie session. Roles in ascending order GUEST < MEMBER < RAIDASSISTENT < RAIDLEADER < ADMIN. A higher role has all powers of a lower role. "Staff" means RAIDASSISTENT and higher.
- Regulatory or privacy constraints: The owner treats the EU GDPR (DSGVO) as applicable and is the responsible person. A legal review of the privacy page is recommended and has not happened.

## Functional Requirements

### 1. Access and roles

- **FR-1.1** The system MUST enforce every permission on the server. Hidden UI controls MUST NOT be the only check. (ADR 010)
- **FR-1.2** The system MUST let a visitor without a session read only these API resources: the raid list, the player list, the poll list, the attendance of a raid, the statistics, the addon list, the addon downloads, and the Rising-Gods character search. (ADR 067, 085, 044; Owner 2026-10-22, OQ-6)
- **FR-1.3** The public player list MUST show only ID, name, guild rank, and active state. It MUST NOT show login names. (ADR 067, 084)
- **FR-1.4** The system MUST let a visitor open the pages `/`, `/anleitung`, `/addons`, `/statistics`, and the privacy page without a session. (ADR 083, 085; Owner 2026-10-22, OQ-6 and OQ-9)
- **FR-1.5** The system MUST require a session for all other reads. (ADR 067)
- **FR-1.6** The system MUST reject a request with a role that is too low with HTTP 403. It MUST reject a request without a session on a protected endpoint with HTTP 401. (ADR 010)
- **FR-1.7** These reads MUST require RAIDASSISTENT or higher: the archive preview, the recruitment template, and the wishlist interests of a raid. (ADR 068, 069)
- **FR-1.8** These reads MUST require ADMIN: the audit log, the registration requests and codes, and the player-account overview. (ADR 074, 084)
- **FR-1.9** The system MUST store the role of each player. Only an ADMIN can change a role. (ADR 010)

### 2. Authentication and sessions

- **FR-2.1** A user MUST log in with a username and a password. (ADR 010)
- **FR-2.2** The system MUST reject a new or changed password with fewer than 12 characters after trimming whitespace. (ADR 046)
- **FR-2.3** The system MUST block login for a username and client address after 5 failed attempts in 15 minutes. (ADR 046)
- **FR-2.4** A successful login MUST start a session that is valid for 30 days. (ADR 010)
- **FR-2.5** The system MUST treat an unknown or expired session as no session, not as an error. (ADR 010, 020)
- **FR-2.6** Logout MUST end the session. (ADR 010)
- **FR-2.7** A logged-in user MUST be able to change the own password. (ADR 046)
- **FR-2.8** The system MUST reject every state-changing request that carries a session cookie and has no valid CSRF token. Missing `Origin` and `Sec-Fetch-Site` headers MUST NOT skip this check. (ADR 046; Owner 2026-10-22, OQ-5)
- **FR-2.9** An inactive account MUST NOT log in. (ADR 020)
- **FR-2.10** At startup, the system MUST create the first ADMIN from the configured bootstrap credentials if the account does not exist. It MUST NOT change an existing account. (ADR 020)
- **FR-2.11** The `/login` page MUST send a logged-in user to the overview. (ADR 081)

### 3. Multi-factor authentication

- **FR-3.1** MFA MUST be optional for each account. An account without MFA MUST log in with the password only. (ADR 046)
- **FR-3.2** A user MUST be able to set up, enable, and disable authenticator-app TOTP with six-digit codes. (ADR 046)
- **FR-3.3** Enabling TOTP MUST show 10 recovery codes one time. Each recovery code MUST work one time only. (ADR 046)
- **FR-3.4** A user MUST be able to register and remove WebAuthn/FIDO2 security keys. (ADR 046)
- **FR-3.5** For an account with MFA, a correct password MUST start an MFA challenge and MUST NOT start a session. The session MUST start only after a valid second factor. (ADR 046)
- **FR-3.6** An MFA challenge MUST expire after 5 minutes. (code; Owner 2026-10-22, review)
- **FR-3.7** The login response MUST list only the factors that the account has configured. The UI MUST show only these factors. (ADR 047)
- **FR-3.8** If the account has a security key and the browser supports WebAuthn, the UI MUST offer the security key first. Code login MUST stay available as a fallback only when the account has TOTP or recovery codes. (ADR 047)

### 4. Registration and character claiming

- **FR-4.1** A visitor MUST be able to request registration only for a Rising-Gods profile character. The system MUST NOT accept a manual character name. (ADR 040)
- **FR-4.2** The system MUST reject a registration or claim request with HTTP 409 `registration_request_exists` when a request for the same profile is PENDING, APPROVED, or CONSUMED. A REJECTED request MUST NOT block a new request. (ADR 055)
- **FR-4.3** An ADMIN MUST be able to list requests, reject a request, create a single-use code for a request, mark a code as sent, and revoke an unused code. (ADR 040)
- **FR-4.4** A registration code MUST expire 14 days after creation. (ADR 040)
- **FR-4.5** Redeeming a new-account code MUST create a MEMBER account, create or link the character, start a session, and consume the code. (ADR 040; Owner 2026-10-22, OQ-3)
- **FR-4.6** A logged-in user MUST be able to request a claim for a Rising-Gods character. Redeeming a claim code MUST link or move the character to the user. (ADR 040)
- **FR-4.7** A claim MUST NOT change existing raid roster snapshots. (ADR 040)
- **FR-4.8** The first Rising-Gods talent build MUST become the main spec. The second build MUST become the second spec when it exists. (ADR 040)
- **FR-4.9** The admin request list MUST show the linked player, login, and date after a code is redeemed. (ADR 084)
- **FR-4.10** The system MUST NOT change the role of existing GUEST accounts automatically. An ADMIN promotes them (FR-1.9). (Owner 2026-10-22, OQ-3)

### 5. Players and accounts

- **FR-5.1** An ADMIN MUST be able to create a player and set the login name, password, and role of a player. (ADR 010)
- **FR-5.2** An ADMIN MUST be able to see for each player the role, login name, characters, and account origin `REGISTERED`, `ADMIN`, or `NONE`. (ADR 084)
- **FR-5.3** An ADMIN MUST be able to merge an active source player into a different target player. The merge MUST require a reason. (ADR 050, 056)
- **FR-5.4** An account merge MUST keep the password, role, and MFA of the target. It MUST end all sessions and remove all MFA data of the source. (ADR 050)
- **FR-5.5** An account merge MUST move all records of the source, including EP transactions, to the target. Then it MUST delete the source permanently. (ADR 056)
- **FR-5.6** An account merge MUST be rejected when both players have an active lock for the same item in the same raid. (ADR 050)
- **FR-5.7** For a duplicate poll vote or quick-add request key, the merge MUST keep the entry of the target. (ADR 056)

### 6. Characters

- **FR-6.1** A player MAY own more than one character. Each character MUST have a class, a main spec, and an optional second spec. (AGENTS.md 4.1)
- **FR-6.2** A character MAY carry a "main" flag. The flag MUST NOT be unique for a player. (AGENTS.md 4.1)
- **FR-6.3** Character names MUST be changeable. The system MUST keep a name history. The system MUST NOT require globally unique character names. (ADR 013)
- **FR-6.4** Interactive creation of a character MUST use a loaded Rising-Gods profile for name, class, and specs. (ADR 042)
- **FR-6.5** The system MUST NOT link one Rising-Gods profile to two players. (ADR 042)
- **FR-6.6** Only an ADMIN MUST be able to create or edit a live character outside raid quick-add. Edits MUST be audited and MUST write the name history. (ADR 032)
- **FR-6.7** An ADMIN MUST be able to move a character to another existing player. The move MUST also move the wishlist and the player reference of every roster row of that character, also in archived raids. (ADR 090, 091)
- **FR-6.8** A character move MUST NOT move loot, EP transactions, item locks, or signups. (ADR 091)
- **FR-6.9** A character move MUST be rejected when the target player already has the character in the same raid roster. (ADR 091)
- **FR-6.10** An ADMIN MUST be able to merge a duplicate character into a correct character of the same player, with a reason. The merge MUST move references and stored name snapshots, store a name alias, and delete the source. (ADR 051, 056)
- **FR-6.11** A character merge MUST be rejected when both characters have records for the same raid or the same wishlist item. (ADR 051)
- **FR-6.12** A character merge MUST NOT change or recreate EP transactions. (ADR 051)
- **FR-6.13** An ADMIN MUST be able to retry the Rising-Gods link of a character. The retry MUST load the profile `eu.rising-gods.<name>` directly and MUST NOT run a list search. (ADR 052)
- **FR-6.14** An ADMIN MUST be able to reload the Rising-Gods profile data of a linked character. (code; Owner 2026-10-22, review)

### 7. Rising-Gods lookup

- **FR-7.1** Interactive lookup MUST accept only Alliance characters of level 80. (ADR 044)
- **FR-7.2** The lookup MUST accept a character name or a Rising-Gods profile URL. (ADR 044)
- **FR-7.3** The system MUST query Rising-Gods only for a user request. It MUST NOT warm a cache in the background. (ADR 049)
- **FR-7.4** The system MUST load the selected profile again before it stores a character. (ADR 044)
- **FR-7.5** When Rising-Gods is unavailable, the system MUST answer with HTTP 502 `rising_gods_unavailable`. User-facing errors MUST stay generic. (ADR 049, 053)
- **FR-7.6** The system MUST limit how many Rising-Gods searches a visitor without a session can start per time period. Logged-in users MUST NOT be limited. The limit value is owned by `SECURITY.md`. (Owner 2026-10-22, review)

### 8. Guild configuration

- **FR-8.1** An ADMIN MUST be able to create and edit guild ranks and raid groups. The system MUST NOT use fixed ranks or groups. (ADR 009)
- **FR-8.2** An ADMIN MUST be able to set the boss order of an instance. The request MUST contain every boss of the instance once, else HTTP 422 `boss_order_invalid`. (ADR 088)
- **FR-8.3** Boss lists and the loot of a raid MUST follow the boss order. Bosses without a position MUST follow in catalog order. (ADR 088)

### 9. Raid series and planning

- **FR-9.1** An ADMIN MUST be able to create and edit a raid series with name, leader, weekdays, week interval, local start time, IANA timezone, name template, next number, instance, size, and planning window. (ADR 021, 034, 078)
- **FR-9.2** The system MUST NOT create a default series or schedule. (ADR 021)
- **FR-9.3** A name template MUST support `{number}`, `{name}`, `{date}`, and `{week}`. An empty template MUST give the series name. (ADR 034)
- **FR-9.4** Recurrence weeks MUST count from the local Monday week of the current raid. (ADR 021)
- **FR-9.5** The local start time MUST stay the same across daylight-saving changes. A local time that does not exist or exists twice MUST give an error. (ADR 021)
- **FR-9.6** Staff MUST be able to plan all missing occurrences of a series up to a chosen date. (ADR 078)
- **FR-9.7** With a planning window above 0 weeks, the system MUST create missing occurrences when the series is saved and when a raid of the series is archived. (ADR 078)
- **FR-9.8** Planning MUST continue from the latest raid of the series, MUST skip past dates, and MUST NOT change existing raids. (ADR 078)
- **FR-9.9** The system MUST reject planning more than 520 weeks ahead. (ADR 078)
- **FR-9.10** Each raid that the system creates from a series MUST open signups at once. (ADR 078)

### 10. Raid lifecycle

- **FR-10.1** Staff MUST be able to create a raid with an instance, a size, an optional series, and an optional scheduled start. (ADR 021)
- **FR-10.2** With a series, the system MUST take the raid name from the series template and MUST ignore a sent name. Without a series, the name MUST be given, else `name_required`. (ADR 035)
- **FR-10.3** A raid MUST have one status: PLANNED, OPEN, COMPLETED, or CANCELLED. (AGENTS.md)
- **FR-10.4** Staff MUST be able to cancel a raid that is not archived, completed, or cancelled. A cancelled raid MUST NOT get EP. Cancelling MUST expire the active item locks of the raid (FR-17.6). (ADR 015; Owner 2026-10-22, OQ-2)
- **FR-10.5** Staff MUST be able to set the series and scheduled start of an active raid. The name MUST be editable only for a raid without a series. (ADR 021)
- **FR-10.6** "Archivieren" MUST be the only final action. It MUST set COMPLETED, the completion time, and the archive time together. (ADR 021)
- **FR-10.7** Archiving MUST be rejected when the raid has no series, no scheduled start, or an incomplete roster. Then no record MUST change. (ADR 021, 022)
- **FR-10.8** In one transaction, archiving MUST award EP (FR-16.2), expire unused item locks, and link or create the next occurrence. (ADR 021)
- **FR-10.9** Archiving MUST be idempotent. A repeated request MUST NOT award EP twice or create a second successor. (ADR 021)
- **FR-10.10** The successor MUST be the first occurrence after the scheduled start and after the archive time. An existing PLANNED or OPEN occurrence MUST be reused without change. A cancelled occurrence MUST be skipped. A completed, archived, or differently linked occurrence MUST give a conflict. (ADR 021, 078)
- **FR-10.11** Staff MUST get an archive preview with each roster player's current EP, the EP to award, and the reason. The preview MUST NOT change data. (ADR 069)
- **FR-10.12** On archive, staff MAY exclude roster players with a DECLINED absence from the EP award. Any other excluded player MUST give HTTP 400 `invalid_exclusion`. (ADR 069, 082)
- **FR-10.13** The archive dialog MUST preselect "no EP" for each declined roster player. (ADR 069)
- **FR-10.14** Archived raids MUST stay visible as history. (ADR 021)
- **FR-10.15** Archiving a raid that was COMPLETED before this rule MUST only set the archive time. It MUST NOT award EP or start a series chain. (ADR 021)
- **FR-10.16** The UI MUST ask for confirmation before it cancels or archives a raid. (ADR 069)

### 11. Current raid and dashboard

- **FR-11.1** "Aktueller Raid" MUST open the newest non-cancelled raid whose real or scheduled start is not in the future, else the next raid. With no raid, the answer MUST be HTTP 404 `no_current_raid`. (ADR 088)
- **FR-11.2** The dashboard MUST show an active raid with a start time in the past as "Läuft" until staff archives it. This MUST NOT be a stored status. (ADR 034)
- **FR-11.3** For MEMBER and higher, the overview MUST show the next four raids that have not started, with the user's status and actions to sign up with the main character, to decline, and to take back a decline. (ADR 081)
- **FR-11.4** The dashboard MUST show the encounters of the configured instance. It MAY show kills only from recorded boss kills (FR-29.4). (ADR 030; Owner 2026-10-22, OQ-7)
- **FR-11.5** The UI MUST NOT show invented data: progression, RSVP rates, notifications, or performance numbers. (ADR 026)

### 12. Signups and absences

- **FR-12.1** Staff MUST be able to open and close signups of a raid. (ADR 033)
- **FR-12.2** Signups MUST be open only when enabled, the raid is active and not archived, and the server time is before the scheduled start. (ADR 033)
- **FR-12.3** MEMBER and higher MUST be able to sign up one or more own characters, each with any valid spec of its class. A signup MUST NOT change the saved specs of the character. (ADR 033)
- **FR-12.4** While signups are open, the player MUST be able to edit or withdraw pending choices. The player MUST NOT edit or withdraw an accepted choice. (ADR 033)
- **FR-12.5** Staff MUST be able to accept one character per player. Acceptance MUST create the roster row with name, class, spec, and the role of the spec. (ADR 033)
- **FR-12.6** Staff MUST be able to switch the accepted character of a player. A stale switch request MUST be rejected. (ADR 033)
- **FR-12.7** Staff MUST be able to accept choices of an active raid after signups close. Cancelled, completed, and archived raids MUST reject acceptance. (ADR 033)
- **FR-12.8** The accepted state MUST come from the roster. A removed roster row MUST show its signup as pending again. (ADR 033, 076)
- **FR-12.9** A player MUST be able to set the own absence to DECLINED or TENTATIVE, with an optional note of at most 200 characters, until the scheduled start. After the start, the answer MUST be HTTP 409 `raid_started`. (ADR 069, 082)
- **FR-12.10** Staff MUST be able to set or clear an absence for any player while the raid is active. (ADR 069, 082)
- **FR-12.11** An absence MUST NOT change the roster, signups, or bench. (ADR 069)
- **FR-12.12** A TENTATIVE absence MUST NOT change the EP rule. (ADR 082)
- **FR-12.13** For members, a planned or open raid page MUST open on "Anmeldungen". For staff, it MUST open on "Aufstellung". (ADR 081)

### 13. Raid roster

- **FR-13.1** Staff MUST be able to add an existing character to the roster of a raid. (ADR 037)
- **FR-13.2** Each roster row MUST store the character name, class, role (TANK, HEAL, DD), and actual spec of that raid. Later changes of the live character MUST NOT change this snapshot. (ADR 022)
- **FR-13.3** When an existing character is selected, the class MUST come from the character and MUST be read-only. Spec, role, and the "Ersatzbank" flag MUST be editable. (ADR 038)
- **FR-13.4** Bench (Ersatzbank) rows MUST follow the same EP rule as other rows. (ADR 016)
- **FR-13.5** Staff MUST be able to create a new character, and optionally a new player without login, from the raid in one step. The new player MUST get the GUEST role. A player name that exists MUST give a conflict. (ADR 037)
- **FR-13.6** Quick-add MUST need a request key. A repeated identical request MUST return the first result. The same key with other content MUST give a conflict. (ADR 037)
- **FR-13.7** Staff MUST be able to remove a roster row from a PLANNED or OPEN, unarchived raid. The row MUST be deleted and its full content MUST be kept in the audit log. (ADR 076)
- **FR-13.8** Removal MUST be rejected with HTTP 409 `attendance_has_loot` while the character holds loot in that raid that is not withdrawn. (ADR 076)
- **FR-13.9** The roster MUST group rows into Tanks, Heilung, Nahkampf, Fernkampf, and "Schaden · unklar" by stored role, class, and spec. (ADR 036)
- **FR-13.10** Roster filters MUST NOT change roster data. (ADR 068)
- **FR-13.11** Staff MUST be able to correct and add roster rows and loot of PLANNED, OPEN, and COMPLETED raids, also when archived. Cancelled raids, and archived raids that are not COMPLETED, MUST reject these writes. (ADR 018, 032)
- **FR-13.12** A correction of an archived raid MUST NOT change the archive times, run the finalization again, create a successor, or revive a used or expired lock. (ADR 032)

### 14. Loot awards

- **FR-14.1** Staff MUST record a loot award with boss, item, recipient character, and method MAIN, SECOND, ENHANCED, or BOE. (ADR 024)
- **FR-14.2** The boss and item MUST be a pair of the bundled catalog for the instance and size of the raid. Missing raid context MUST give HTTP 409 `catalog_context_required`. An invalid pair MUST give HTTP 400 `catalog_item_invalid`. (ADR 027)
- **FR-14.3** The recipient MUST be a character in the roster of that raid. (ADR 024)
- **FR-14.4** MAIN and ENHANCED MUST count as main-spec loot. SECOND and BOE MUST NOT. (ADR 024)
- **FR-14.5** An ENHANCED award MUST debit X = min(balance, 50) from the recipient in the same transaction. It MUST require X ≥ 1. Staff MUST NOT choose or split the amount. (ADR 023, 024)
- **FR-14.6** A new ENHANCED award MUST carry a nonblank `request_id`. An identical retry MUST return the stored award. The same key with other content MUST give a conflict. (ADR 024)
- **FR-14.7** When an active item lock matches the item, the award MUST go to the lock owner as MAIN for 0 EP, and the lock MUST become USED. (ADR 012, 017)
- **FR-14.8** A correction that keeps an ENHANCED recipient and method MUST keep the original cost. A change away from ENHANCED MUST refund the cost. A change of the ENHANCED recipient MUST refund the old winner and charge min(new balance, 50). (ADR 024)
- **FR-14.9** Staff MUST be able to withdraw a manual loot award. A reason MUST be required only for an archived raid (FR-26.5). The award MUST stay stored as withdrawn. (ADR 032; Owner 2026-10-22, OQ-1)
- **FR-14.10** Cancelled raids MUST reject loot writes. (ADR 032)
- **FR-14.11** The UI MUST mark an item that was already awarded in the raid. It MUST allow a second award of the same item. (ADR 069)

### 15. Unclaimed loot

- **FR-15.1** Staff MUST be able to record an item that went to nobody, with outcome DISENCHANTED, BANK, or NOBODY, holder name, optional boss of the raid instance, and notes. (ADR 089)
- **FR-15.2** Unclaimed loot MUST NOT affect EP, loot counts, tokens, or item locks. (ADR 089)
- **FR-15.3** Staff MUST be able to remove an entry. Removal MUST archive the entry. (ADR 089)
- **FR-15.4** Cancelled raids MUST reject unclaimed loot. Archived raids MUST require a reason for add and remove. (ADR 089)

### 16. Enhance Points

- **FR-16.1** The EP balance MUST be the sum of the EP transactions of the player. The system MUST NOT change or delete an EP transaction. A correction MUST add a new transaction. (ADR 004)
- **FR-16.2** Archiving MUST award +10 EP one time to each roster player who got no main-spec loot in the raid, except players excluded under FR-10.12. (ADR 004, 016, 021)
- **FR-16.3** The website MUST NOT generate rolls or offer a spend action, amount selector, or roll button. (ADR 023)
- **FR-16.4** The UI MUST show the balance, the shifted roll range 1+X to 100+X with X = min(balance, 50), and the history. (ADR 023, 026)
- **FR-16.5** A correction of a completed raid MUST add only the change in EP eligibility: +10 when a player becomes eligible, −10 when a player is no longer eligible, limited by FR-16.6. (ADR 018)
- **FR-16.6** A correction MUST NOT make an EP balance lower than 0. It MUST take only the points that the player still has. The points not taken MUST NOT be taken later. (Owner 2026-10-22, review)
- **FR-16.8** Before staff save a correction that FR-16.6 limits, the system MUST warn them with the player and the number of points not taken. The audit entry MUST record that number. (Owner 2026-10-22, review)
- **FR-16.7** The system MUST NOT recalculate existing ledger history under new rules. (AGENTS.md)

### 17. Item locks

- **FR-17.1** A MEMBER or higher MUST be able to create and cancel only own item locks. No role MAY act on the locks of another player. (ADR 017, 020)
- **FR-17.2** Creating a lock MUST require a balance of at least 150 EP. A lock MUST cost 0 EP. (ADR 012)
- **FR-17.3** A player MAY hold locks on different items. A raid MUST have at most one active lock for the same item. A competing lock MUST be rejected. (ADR 012, 017)
- **FR-17.4** Item identity MUST use the trimmed item ID when both records have one, else the trimmed item name. (ADR 017)
- **FR-17.5** Completed, cancelled, and archived raids MUST reject new locks. (ADR 017)
- **FR-17.6** Archiving or cancelling a raid MUST expire each unused lock of the raid without EP loss. (ADR 012, 021; Owner 2026-10-22, OQ-2)
- **FR-17.7** Locks of raids cancelled before 2026-10-22 MUST NOT be changed. (Owner 2026-10-22, OQ-2)

### 18. Armor tokens

- **FR-18.1** Token information MUST come from loot awards of the items "Weiheabzeichen des Beschützers", "Weiheabzeichen des Bezwingers", and "Weiheabzeichen des Eroberers". The system MUST NOT store a token balance. (ADR 041)
- **FR-18.2** The player profile MUST show tokens as a filter of the loot of the player. (ADR 030, 041)

### 19. Wishlist

- **FR-19.1** The owner of a character, MEMBER or higher, MUST be able to add, edit, and remove wishlist items of that character. No other player MAY change them. (ADR 070)
- **FR-19.2** A wishlist item MUST have a priority, notes, a spec MAIN or SECOND (default MAIN), and an "enhanced" flag that only MAIN allows. (ADR 070)
- **FR-19.3** The owner and staff MUST be able to read a wishlist. An unknown character MUST give HTTP 404. (ADR 069)
- **FR-19.4** Staff MUST be able to read the wishlist interests of all roster characters of a raid. (code; Owner 2026-10-22, review)
- **FR-19.5** "Meine Wunschliste" MUST show the characters of a MEMBER, with the main character first. (ADR 081)

### 20. Addon exchange

- **FR-20.1** An authenticated user MUST be able to get the invite export `RS2;INV` of a raid roster with current character names. Bench rows MUST be included only on request. (ADR 045, 070)
- **FR-20.2** The invite header MUST contain the raid date and start time in the series timezone when the raid has a series and a scheduled start. Else it MUST have five fields. (ADR 085)
- **FR-20.3** Roster and loot imports MUST be a preview followed by an explicit apply, by staff, for PLANNED or OPEN raids that are not archived. The preview MUST NOT write. (ADR 045)
- **FR-20.4** The imports MUST read RS1 (`|`-separated) and RS2 (`;`-separated). (ADR 070)
- **FR-20.5** Roster apply MUST add only selected known characters through the normal roster path. It MUST NOT create unknown characters. The preview MUST hand unknown names to quick-add. (ADR 045)
- **FR-20.6** Loot apply MUST write each line through the normal loot path. Staff MUST choose the method and MAY change the recipient to any roster character. (ADR 045)
- **FR-20.7** Each imported loot line MUST use a raid-scoped request ID from raid, recorder, recording start, and entry ID. A repeated import MUST create nothing new. (ADR 045)
- **FR-20.8** The default method MUST be ENHANCED for an enhanced range, else SECOND for armor tokens and second-spec rolls, else MAIN. (ADR 070)
- **FR-20.9** `parked` and `deleted` lines MUST be shown and MUST NOT be imported (`not_importable`). (ADR 070)
- **FR-20.10** `disenchanted` and `bank` lines MUST be imported as unclaimed loot, once per line key. (ADR 089)
- **FR-20.11** An import MUST NOT withdraw or move an existing award. The preview MUST warn when the export names another recipient. (ADR 045, 070)
- **FR-20.12** When the addon range and the account balance disagree, the preview MUST warn and the system MUST debit the account value. (ADR 070)
- **FR-20.13** The owner and staff MUST be able to get the WL2 wishlist export with the current EP balance, in priority order. (ADR 070)

### 21. Addon downloads

- **FR-21.1** The system MUST list the bundled addons RaidLedger and RaidWishlist with name, version, file name, size, and download URL. (ADR 085)
- **FR-21.2** A download MUST return a zip with one top-level folder named after the addon. An unknown name MUST give HTTP 404 `addon_not_found`. (ADR 085)
- **FR-21.3** The system MUST provide German pages `/addons`, `/addons/raidledger`, and `/addons/raidwishlist`. (ADR 085)

### 22. Shadowmourne

- **FR-22.1** The system MUST keep one shared list of entries with character, priority, stage, shard count, and shard target. (ADR 086)
- **FR-22.2** Stages MUST be PREQUEST, SOULS, UNHOLY, BLOOD, FROST, SHARDS, FINAL. Staff MAY set any stage. (ADR 086)
- **FR-22.3** Only Krieger, Paladin, and Todesritter MUST be accepted. A character MUST be active on the list only once. (ADR 086)
- **FR-22.4** The shard target MUST default to 50. The count MUST be nonnegative and MUST NOT exceed the target. (ADR 086)
- **FR-22.5** Staff MUST be able to add an entry at the end or at a position and move an entry. Priorities MUST be renumbered 1..n. (ADR 086)
- **FR-22.6** "Fertig" MUST require the shard target and MUST move the entry to history. "Entfernen" MUST archive the entry. (ADR 086)
- **FR-22.7** For UNHOLY, BLOOD, FROST, and SHARDS in a raid, the system MUST suggest the highest-priority roster character on that stage. (ADR 086, 087)
- **FR-22.8** Staff MAY choose a roster character on that stage, or a roster Krieger, Paladin, or Todesritter not on the active list. Other choices MUST be rejected. (ADR 087)
- **FR-22.9** Cancelled and archived raids MUST reject task choices. A choice MUST count only while the character is in the roster. (ADR 086, 087)
- **FR-22.10** Every authenticated user MUST be able to read the list and the raid plan. (ADR 086)

### 23. Recruitment text

- **FR-23.1** Staff MUST be able to read and save a recruitment template for a planned or open raid: one per series, else one per raid. (ADR 068)
- **FR-23.2** The template text MUST have at most 2000 characters and a 5×5 target setup. (ADR 068)
- **FR-23.3** The text MUST support `{{SETUP}}`, `{{SETUP_KURZ}}`, `{{TANK_SPECS}}`, `{{HEAL_SPECS}}`, `{{MDD_SPECS}}`, and `{{RDD_SPECS}}`, computed from the saved roster. (ADR 080)
- **FR-23.4** The server MUST reject a target-setup slot value that is not a known class-and-spec value or "Irgendein" placeholder. (Owner 2026-10-22, OQ-4)

### 24. Polls

- **FR-24.1** Staff MUST be able to create and close polls. (ADR 010)
- **FR-24.2** A poll MUST be TEXT (free answer) or CHOICE (ordered options, single or multiple selection). (ADR 039)
- **FR-24.3** A MEMBER or higher MUST have one vote per poll and MAY change it while the poll is open. (code; Owner 2026-10-22, review)
- **FR-24.4** A vote on a closed poll or after its closing time MUST give HTTP 409 `poll_closed`. (code)
- **FR-24.5** A CHOICE vote MUST reference only configured options. Duplicate or too many selections MUST be rejected. (ADR 039)
- **FR-24.6** A poll write without a session MUST give HTTP 401 `login_required`, like every other protected endpoint. (Owner 2026-10-22, OQ-10)

### 25. Loot catalog

- **FR-25.1** The catalog MUST come only from the bundled file. The system MUST NOT offer catalog upload or edit. (ADR 027)
- **FR-25.2** Every authenticated role MUST be able to read the catalog. (ADR 027)
- **FR-25.3** The catalog MUST group drops into NORMAL, HEROIC, and HARDMODE and MUST mark unproven drops as conditional. (ADR 027)
- **FR-25.4** The system MUST NOT rewrite historical free-text or imported loot to match the catalog. (ADR 027)

### 26. Audit log

- **FR-26.1** Every website write by a user MUST write one audit entry with actor, entity, action, and before and after snapshots. (ADR 077)
- **FR-26.2** Where the write is transactional, the entry MUST be part of the same transaction. (ADR 077)
- **FR-26.3** Snapshots MUST NOT contain passwords, password hashes, TOTP secrets, recovery codes, registration codes, or raw Rising-Gods profile data. (ADR 077)
- **FR-26.4** Login, logout, and MFA verification MUST NOT write audit entries. (ADR 077)
- **FR-26.5** Changes to an archived raid MUST require a nonblank reason. Changes to unarchived raids and live characters MUST NOT require a reason. (ADR 032)
- **FR-26.6** An ADMIN MUST be able to read the audit log, newest first, with German labels and the raw snapshots in a collapsed section. The log MUST be read-only. (ADR 074, 075)

### 27. Statistics

- **FR-27.1** The statistics page MUST show raids by status, class distribution, historical roles, loot distribution, most attendances, and most loot entries. (Owner 2026-10-22, OQ-6)
- **FR-27.2** Visitors without a session MUST be able to see the statistics, including loot totals per player. The public statistics MUST NOT show item lists or EP balances. (Owner 2026-10-22, OQ-6)

### 28. Guide, version, and language

- **FR-28.1** The system MUST provide the guide `/anleitung` with chapters for each role and generated screenshots. (ADR 083)
- **FR-28.2** The navigation MUST show the application version. A changelog page MUST list user-facing changes in German. (ADR 056)
- **FR-28.3** All user-facing text MUST be German. (AGENTS.md)
- **FR-28.4** The UI MUST show times in the user's local timezone. (ADR 003)

### 29. Boss kills

- **FR-29.1** A raid MUST have a list of killed bosses. Each entry MUST name a boss of the raid instance and the difficulty of that kill. (Owner 2026-10-22, OQ-7)
- **FR-29.2** Difficulty MUST be recorded per boss, not per raid. (Owner 2026-10-22, OQ-7)
- **FR-29.3** A new loot award of a boss MUST take its difficulty from the kill entry of that boss. (Owner 2026-10-22, OQ-7)
- **FR-29.4** The dashboard MAY show progress from recorded kills. It MUST NOT show kills that were not recorded. (Owner 2026-10-22, OQ-7)
- **FR-29.5** Staff MUST be able to record and remove a kill. These writes MUST follow the raid-state rules of FR-13.11, the reason rule of FR-26.5, and MUST be audited. (Owner 2026-10-22, OQ-11)
- **FR-29.6** A kill MUST have one difficulty: NORMAL, HEROIC, or HARDMODE. The system MUST offer only the difficulties that the catalog has for that boss and raid size. A raid MUST have at most one kill per boss. (Owner 2026-10-22, OQ-11)
- **FR-29.7** When staff record loot of a boss that has no kill in the raid, the system MUST ask for the difficulty and MUST create the kill in the same transaction as the loot. (Owner 2026-10-22, OQ-11)
- **FR-29.8** The addon loot import MUST ask for the difficulty once per boss without a kill, and MUST create the kill as in FR-29.7. (Owner 2026-10-22, OQ-11)
- **FR-29.9** Removal of a kill MUST be rejected while the raid has loot of that boss that is not withdrawn. (Owner 2026-10-22, OQ-11)
- **FR-29.10** The system MUST NOT create kills for loot recorded before this feature. Such raids MUST show that no kills are recorded. (Owner 2026-10-22, OQ-11)
- **FR-29.11** RaidLedger MUST NOT be changed for kills now. A kill export is out of scope. (Owner 2026-10-22, OQ-11)

### 30. Privacy

- **FR-30.1** The system MUST provide a German privacy page (Datenschutzerklärung) that is readable without login. It MUST name the owner as the responsible person with a contact address. The name and address MUST come from configuration, not from the repository. (Owner 2026-10-22, OQ-9 and OQ-12)
- **FR-30.2** An ADMIN MUST be able to anonymise a player. Anonymisation MUST delete the login, password, sessions, MFA data, and Rising-Gods profile data, and MUST replace the player name with the name of the player's main character (see OQ-13). (Owner 2026-10-22, OQ-9 and OQ-12)
- **FR-30.5** A person MUST ask for anonymisation through the contact on the privacy page. Only an ADMIN MUST be able to do it. Members MUST NOT get a self-service delete. (Owner 2026-10-22, OQ-12)
- **FR-30.3** Anonymisation MUST keep raid rosters, loot, EP transactions, and character names in raid history. (Owner 2026-10-22, OQ-9)
- **FR-30.4** Audit entries MUST stay without a time limit. They MUST NOT show the name of an anonymised player. (Owner 2026-10-22, OQ-9 and OQ-12)

## Non-Functional Requirements

- **NFR-1.1** The system MUST NOT start when the configuration, a migration, the bootstrap admin, the bundled catalog, or the addon bundle is invalid. (ADR 020, 027, 085)
- **NFR-1.2** In production, the system MUST NOT start with default or missing bootstrap credentials. (ADR 020)
- **NFR-1.3** In production, the application MUST listen only on a loopback address behind a TLS reverse proxy. (ADR 070)
- **NFR-2.1** The system MUST take a verified database snapshot before each deployment. (ADR 071, 072)
- **NFR-2.2** The system MUST keep 48 hourly, 32 daily, 13 weekly, and 25 monthly database snapshots. A weekly job MUST verify the newest snapshot. (ADR 070)
- **NFR-2.3** An encrypted copy of the newest verified snapshot MUST leave the server every day for storage that the owner controls. The maximum data loss is therefore one day. The storage and the key holder are TO BE DECIDED (OQ-14). (ADR 070; Owner 2026-10-22, OQ-8)
- **NFR-2.5** A restore from the off-server copy MUST be tested every month. (Owner 2026-10-22, OQ-8)
- **NFR-3.1** The system MUST cache Rising-Gods name searches for 12 hours and MAY serve them stale for up to 7 days while it refreshes. The cache MUST hold at most 2000 searches. (ADR 044)
- **NFR-3.2** Parallel identical Rising-Gods lookups MUST share one upstream request. (ADR 044)
- **NFR-4.1** The server MUST reject a request body larger than 1 MiB. (code)
- **NFR-5.1** The production host MUST deploy only from the protected `main` branch, check hourly, and restart for kernel updates at 03:00 Europe/Berlin only when a reboot is required. (ADR 072, 073)

## Open Questions

The owner's question and review pass on 2026-10-22 resolved OQ-1 to OQ-12. Their answers are in
the requirements marked "Owner 2026-10-22". NFR-2.4 (delete the pre-migration-066 backup) is
done and was removed. IDs are not reused.

- **OQ-13** Anonymisation (FR-30.2): Which name applies when the player has several main characters (FR-6.2) or none?
- **OQ-14** Off-server backup (NFR-2.3): Which storage, and who holds the encryption key?
