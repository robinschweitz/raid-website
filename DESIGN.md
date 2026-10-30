---
version: alpha
name: Bratwurst des Schicksals — Raid-Website
description: Dark, icy Icecrown-Citadel-inspired guild raid tool. Navy and near-black glass panels, cyan and silver accents, class colours for characters, dense readable data.
colors:
  primary: "#86d9f2"
  primary-bright: "#7ad7ff"
  primary-highlight: "#d7f4ff"
  on-primary: "#06111c"
  background: "#080f1c"
  surface: "#101d2e"
  surface-card: "#0f1a2b"
  surface-raised: "#16273b"
  surface-sidebar: "#101d30"
  on-surface: "#dce6f0"
  on-surface-strong: "#e4eef7"
  on-surface-muted: "#9eafc3"
  outline: "#2a4057"
  outline-control: "#5989a3"
  nav: "#a6b9cc"
  nav-active: "#b3eaff"
  success: "#8bd4b0"
  info: "#8acdea"
  warning: "#ffd27a"
  warning-outline: "#c99a3d"
  danger: "#ff969c"
  danger-surface: "#251f2f"
  notice-surface: "#0c2a3c"
  on-notice: "#c7efff"
  class-deathknight: "#ed6b80"
  class-druid: "#ffab65"
  class-hunter: "#abd473"
  class-mage: "#80daf3"
  class-paladin: "#f5a9d0"
  class-priest: "#edf3fa"
  class-shaman: "#729cff"
  class-rogue: "#fff19c"
  class-warlock: "#b8a6ef"
  class-warrior: "#d4b38f"
typography:
  display:
    fontFamily: "Georgia, 'Times New Roman', serif"
    fontSize: 42px
    fontWeight: 400
  headline-lg:
    fontFamily: "Georgia, 'Times New Roman', serif"
    fontSize: 2.2rem
    fontWeight: 400
    letterSpacing: 0.01em
  brand:
    fontFamily: "Georgia, serif"
    fontSize: 24px
    fontWeight: 400
    letterSpacing: 0.025em
  headline-md:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif"
    fontSize: 1.15rem
    fontWeight: 700
  title-sm:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif"
    fontSize: 1rem
    fontWeight: 700
  body-md:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif"
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.55
  body-sm:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif"
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.5
  label-md:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif"
    fontSize: 0.85rem
    fontWeight: 400
  label-sm:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif"
    fontSize: 12px
    fontWeight: 400
  caption:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif"
    fontSize: 12px
    fontWeight: 650
    letterSpacing: 0.14em
  stat:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif"
    fontSize: 2rem
    fontWeight: 700
rounded:
  xs: 3px
  sm: 4px
  md: 6px
  lg: 8px
  full: 999px
spacing:
  xs: 4px
  sm: 8px
  md: 12px
  lg: 16px
  xl: 20px
  xxl: 24px
components:
  card:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.sm}"
    padding: 1rem 1.25rem
  subcard:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.xs}"
    padding: 0.5rem 1rem
  button:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.on-surface}"
    typography: "{typography.label-md}"
    rounded: "{rounded.xs}"
    padding: 0.4rem 0.8rem
    height: 36px
  button-primary-action:
    backgroundColor: "{colors.primary-bright}"
    textColor: "{colors.on-primary}"
    typography: "{typography.label-md}"
    rounded: "{rounded.xs}"
    padding: 0.4rem 0.8rem
    height: 36px
  input:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.on-surface}"
    typography: "{typography.label-md}"
    rounded: "{rounded.xs}"
    padding: 0.4rem 0.6rem
    height: 36px
  field-label:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.on-surface-muted}"
    typography: "{typography.label-md}"
  badge:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.on-surface-muted}"
    typography: "{typography.label-sm}"
    rounded: "{rounded.xs}"
    padding: 0.1rem 0.5rem
  badge-ok:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.success}"
    typography: "{typography.label-sm}"
    rounded: "{rounded.xs}"
  badge-warn:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.warning}"
    typography: "{typography.label-sm}"
    rounded: "{rounded.xs}"
  badge-bad:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.danger}"
    typography: "{typography.label-sm}"
    rounded: "{rounded.xs}"
  sidebar:
    backgroundColor: "{colors.surface-sidebar}"
    textColor: "{colors.nav}"
    width: 244px
  nav-link:
    backgroundColor: "{colors.surface-sidebar}"
    textColor: "{colors.nav}"
    padding: 11px 12px
  nav-link-active:
    backgroundColor: "{colors.surface-sidebar}"
    textColor: "{colors.nav-active}"
    padding: 11px 12px
  sidebar-caption:
    backgroundColor: "{colors.surface-sidebar}"
    textColor: "{colors.on-surface-muted}"
    typography: "{typography.caption}"
  success-message:
    backgroundColor: "{colors.notice-surface}"
    textColor: "{colors.on-notice}"
    rounded: "{rounded.sm}"
    padding: 8px 8px 8px 14px
  error-message:
    backgroundColor: "{colors.danger-surface}"
    textColor: "{colors.danger}"
    padding: 9px 12px
  link:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.primary}"
  stat-value:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.primary}"
    typography: "{typography.stat}"
  page-title:
    backgroundColor: "{colors.background}"
    textColor: "{colors.on-surface-strong}"
    typography: "{typography.headline-lg}"
  section-title:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.on-surface-strong}"
    typography: "{typography.headline-md}"
  raid-banner-title:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface-strong}"
    typography: "{typography.display}"
  brand-mark:
    backgroundColor: "{colors.surface-sidebar}"
    textColor: "{colors.on-surface-strong}"
    typography: "{typography.brand}"
  table-cell:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.on-surface}"
    typography: "{typography.body-md}"
    padding: 0.65rem 0.75rem
  help-text:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.on-surface-muted}"
    typography: "{typography.body-sm}"
  info-text:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.info}"
    typography: "{typography.body-md}"
  focus-ring:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.primary}"
  card-title:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.on-surface-strong}"
    typography: "{typography.title-sm}"
  badge-warn-outline:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.warning-outline}"
  primary-action-highlight:
    backgroundColor: "{colors.primary-highlight}"
    textColor: "{colors.on-primary}"
  class-label-deathknight:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.class-deathknight}"
  class-label-druid:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.class-druid}"
  class-label-hunter:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.class-hunter}"
  class-label-mage:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.class-mage}"
  class-label-paladin:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.class-paladin}"
  class-label-priest:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.class-priest}"
  class-label-shaman:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.class-shaman}"
  class-label-rogue:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.class-rogue}"
  class-label-warlock:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.class-warlock}"
  class-label-warrior:
    backgroundColor: "{colors.surface-card}"
    textColor: "{colors.class-warrior}"
---

# DESIGN.md — raid-website

This file owns the visual and frontend design language: brand, colour, type, layout, component
conventions, accessibility targets, and security-relevant interaction and rendering. Behaviour is
owned by `REQUIREMENTS.md`; this file references `FR-*` IDs instead of restating them. The tokens
above describe the CSS in `web/src/index.css` and `web/src/secondary-pages.css` as of v1.0.27.
`UNKNOWN` marks missing facts, `TO BE DECIDED` marks open choices, `ASSUMPTION` marks inferences.
Rules marked "Owner 2026-10-22" are decisions that the code does not follow yet; where they
change an older ADR, the implementing change adds a superseding ADR.

## Overview

A raid tool for one WotLK guild, "Bratwurst des Schicksals". The mood is Icecrown Citadel at
night: deep navy and near-black panels over a dimmed citadel photo, icy cyan and silver accents,
restrained frost borders and glow, and modern, dense, readable data (ADR 026). It feels like a
well-kept guild ledger, not a casino and not a game launcher.

- **Brand mark:** the original crest `web/public/guild-crest.svg` (a shield with a frost star and
  blade, vector shapes only, `#091725` / `#91bdcf` / `#79cce5` / `#b4d6e5` / `#ebf8ff`). It sits
  above the guild name in the sidebar and on the login card. No new logo is introduced.
- **Data first:** real raid, roster, loot, EP and poll data come before decoration. The UI shows
  no invented numbers (FR-11.5).
- **Class identity:** characters always carry their class colour and class emblem.
- **Language:** all visible text is German (FR-28.3); umlauts are real Unicode characters, never
  HTML entities inside JavaScript strings (ADR 031).

## Colors

One dark theme only (`color-scheme: dark`); there is no light mode.

- **Primary `#86d9f2`** (icy cyan): links, active navigation edge, focus ring, key figures.
  `primary-bright` `#7ad7ff` to `primary-highlight` `#d7f4ff` is the gradient of the single most
  important action on a panel, always with `on-primary` `#06111c` text.
- **Surfaces:** `background` `#080f1c` under a darkened `bg-icc.png`; cards are translucent
  `rgba(16, 29, 46, 0.82)` with a 10px backdrop blur. `surface-card` `#0f1a2b` is that card
  rendered over `background` and is the value contrast is measured against. `surface-raised`
  `#16273b` fills buttons, inputs, badges and subcards.
- **Text:** `on-surface` `#dce6f0` for body, `on-surface-strong` `#e4eef7` for headings,
  `on-surface-muted` `#9eafc3` for labels, hints and secondary data.
- **Status:** `success` `#8bd4b0` (bestätigt, ok), `warning` `#ffd27a` with `warning-outline`
  `#c99a3d` (Unsicher, amber), `danger` `#ff969c` on `danger-surface` (errors, Abgesagt),
  `info` `#8acdea`. Status colour is never the only signal: every badge carries a German word.
- **Class colours:** the ten WotLK classes, applied as text, borders and emblem glow through the
  `class-*` CSS classes (`web/src/characterTheme.ts`). Unknown classes fall back to neutral text.
- **Lines:** `outline` `#2a4057` for card edges, table rules and dividers (decorative only).
  `outline-control` `#5989a3` for the edges of every input, select, textarea and button, so the
  control meets 3:1 (Owner 2026-10-22, DQ-1).
- No gold-heavy styling (ADR 026). Gold-amber appears only as the `warning` status.

## Typography

- **Serif for identity:** Georgia for the brand name, page titles (`headline-lg`) and the raid
  banner title (`display`), weight 400, slight letter spacing.
- **Fluid title sizes:** the token sizes are the maximum. In CSS, `display` is
  `clamp(28px, 4vw, 42px)` and `headline-lg` is `clamp(1.65rem, 3vw, 2.2rem)`, so titles shrink on
  narrow screens.
- **System sans for data:** `system-ui` stack for everything else. Body is 14px with 1.55 line
  height; tables, forms and badges step down to `label-md`, `body-sm` and `label-sm`.
- **Caption:** sidebar captions and eyebrows are 12px, weight 650, uppercase, letter spacing
  0.14em.
- **Minimum size:** no text is smaller than 12px, including badges, captions and table notes
  (Owner 2026-10-22, DQ-3).
- **Figures:** big numbers (`stat`) are 2rem bold in `primary`.
- No web fonts are loaded; the page uses only fonts on the device.

## Layout

- **Desktop (above 850px):** a fixed 244px sidebar on the left (crest, guild name, navigation
  grouped under captions, version link at the bottom) and the content to its right.
- **Mobile (850px and below):** the sidebar becomes a top bar, and a fixed bottom companion
  navigation shows the main destinations with short labels (for example "Aktuell", "Hilfe").
  Safe-area insets are respected.
- **Grids:** card grids use `repeat(auto-fit, minmax(280px, 1fr))` with a 1.25rem gap. The
  dashboard uses two columns (agenda and points, history and polls) that stack on narrow screens.
- **Raid roster:** four role columns (Tanks, Heilung, Nahkampf, Fernkampf) on wide screens, two
  on medium, one on mobile; the Ersatzbank is a separate section (ADR 036, ADR 038).
- **Raid tabs:** a tab row on desktop; a two-column grid on small screens, never a sideways
  scrolling strip (ADR 069).
- **Further breakpoints:** 600px stacks inline forms to full width; 1250px and 1600px widen
  dashboard columns. Wide tables scroll inside `.table-scroll`, never the whole page.
- **Forms before lists:** "add" forms sit as collapsed sections above the list they add to, so
  they are reachable without scrolling (ADR 069).
- **Spacing:** values cluster on the 4/8/12/16/20/24px scale above; rem values (0.25–1.25rem)
  are equivalent at the 16px root.

## Elevation & Depth

Depth comes from glass, not from stacked shadows.

- **Card:** translucent fill, 10px backdrop blur, 1px `rgba(42, 64, 87, 0.6)` border, an inner
  top highlight and a soft `0 8px 32px` black shadow. Hover adds a faint cyan glow
  (`rgba(134, 217, 242, 0.12)`) and a brighter border.
- **Sidebar:** vertical navy gradient with 12px blur and a right border.
- **Raid banner:** layered radial and linear navy gradients, a lighter top border, and a left
  fade so text stays readable over the artwork.
- **Sticky layers:** success messages (z 20) and loot boss headings stick while scrolling;
  roster column headings do not (ADR 069).

## Shapes

- Controls (buttons, inputs, badges, subcards): `xs` 3px corners.
- Cards, banners, notices: `sm` 4px. Larger tiles: `md` 6px to `lg` 8px.
- Pills and avatars: `full`. Emblems and crests keep their own silhouette.
- Active navigation links are flat on the left (`0 3px 3px 0`) with a 2px cyan left edge.

## Components

All controls are native HTML elements (`button`, `input`, `select`, `textarea`, `table`,
`dialog`-free confirmation). No UI or component library is used (ADR 026, ADR 068).

- **Button:** raised surface, 36px minimum height, border turns cyan on hover; disabled is 50%
  opacity with a not-allowed cursor. While a request runs, the label says what is happening and
  the button is disabled, so a double click cannot submit twice (ADR 020).
- **Primary action:** at most one per panel, cyan-to-white gradient with dark bold text.
- **Input and select:** raised surface, label above the field in `on-surface-muted`. Pickers for
  characters, items and recipients are native inputs with suggestions, full width, with a clear
  button directly next to them (ADR 068, ADR 069).
- **Badge:** small bordered label; `ok`, `warn`, `bad` variants change text and border colour.
- **Success message:** dismissible notice in `notice-surface` with a cyan left edge; sticky on
  secondary pages; cleared when the raid tab changes (ADR 069).
- **Error message:** one shared component for every error: `danger` text on `danger-surface`
  with a red left edge, long text wraps, always `role="alert"` (Owner 2026-10-22, DQ-5). Server error codes
  are shown as German sentences (ADR 069).
- **Card / subcard:** content container; records are edited in place inside their own card with
  Save and Cancel, never in a detached editor (ADR 032).
- **Table:** full width, rule under each row, left-aligned; `small` variant for dense lists.
- **Class emblem:** the ten Blizzard class icons, stored in the repository and served by the
  site itself from `web/public/class-icons/`; no class image is loaded from another server
  (Owner 2026-10-22, DQ-2, ADR 104). Unknown classes show the neutral fallback.
- **Character chip:** name in class colour plus class emblem, owner and spec as muted
  text; `#ID` only when two names would look the same (ADR 069).
- **Empty state:** short muted sentence; for roles that can act, it names the action to take
  (ADR 069).
- **EP card:** balance, the shifted roll range preview and history; never a roll, spend or amount
  control (FR-16.3, FR-16.4).
- **Loot catalog:** read-only; catalog provenance and raw source variants are secondary detail,
  hidden from the ordinary item view (ADR 030).
- **Copy box:** for addon exports, numbered steps and one "Export kopieren" button; if the
  clipboard is blocked, the text is shown selected for manual copying (ADR 081).

- **Loot entry:** one form for awards and for items that went to nobody. "Vergabemethode" comes before the recipient and also offers Entzaubern, Gildenbank and Wollte niemand; Entzaubern asks for the disenchanting roster character in the recipient field, Gildenbank and Wollte niemand replace that field with an optional "Bei" field (FR-15.1, ADR 107).
- **Install hint:** one thin line under the top bar in the `info` colour, styled like the
  account-security reminder: phone icon, a bold sentence, muted detail, the browser's
  "Installieren" button where offered, and a quiet close button (FR-28.5, ADR 101). App icons
  are PNGs of the crest on `background` (`web/public/app-icon-*.png`, `apple-touch-icon.png`).
- **Push opt-in:** a subcard "Benachrichtigungen" in the own account tab, above the MFA card,
  for MEMBER and higher: one muted sentence on what is sent and that it names no content, the
  device's state ("aktiv"/"aus"), and one ordinary button "Benachrichtigungen aktivieren" or
  "… abschalten". On iPhone/iPad outside the installed app it shows only the install note; a
  blocked permission is explained, not hidden. It is not shown when the server has no VAPID
  key (FR-32, ADR 102). Notifications themselves use the title "RAID", the 192px app icon,
  and a fixed German sentence (FR-32.5).
- **New screens:** the boss kill list and difficulty prompt (FR-29), the privacy page and the
  anonymise action (FR-30), the EP-floor warning (FR-16.8), the forced Admin MFA setup (FR-3.10), and the password reset
  with a code (FR-5.8) are designed in their own
  implementation issue, built only from the components above, and reviewed through the
  screenshots in the pull request (Owner 2026-10-22, DQ-4).

## Do's and Don'ts

- Do show only stored data; show an explicit loading, empty or error state instead of a guess.
- Do keep one primary action per panel and keep destructive actions visually quiet.
- Do keep the crest, class colours and frost accents original; reuse existing tokens.
- Do serve every image from the site itself; don't load images from other servers. The only
  exception is the Wowhead item icons of the loot catalog on `wow.zamimg.com` (ADR 104).
- Don't reproduce a screenshot or game art as the page background or as UI chrome.
- Don't add fake progression, RSVP percentages, notifications or performance statistics.
- Don't add a CSS framework, component library, icon font or web font without owner approval.
- Don't hide information from a role only by styling; permissions are the server's job (SC-001).

## Accessibility

Target: WCAG 2.2 level AA.

- **Contrast (checked 2026-10-22 against `surface-card`, `surface-raised` and `surface-sidebar`):**
  all text tokens pass 4.5:1 (lowest: `class-deathknight` 5.8:1, `class-shaman` 6.6:1,
  `on-surface-muted` 6.8:1). The focus ring passes 3:1 (11.0:1). Form-control edges in
  `outline-control` pass 3:1 (4.0:1 against `surface-raised`, 4.6:1 against `surface-card`).
  Every control edge uses `outline-control`; the former `outline` edge was 1.4:1 to 1.6:1.
  ASSUMPTION: the translucent cards stay at or darker than `surface-card` over every part of the
  background photo.
- **Focus:** every focusable element shows a 2px `primary` outline with 3px offset
  (`:focus-visible`). A skip link moves to the main content.
- **Targets:** buttons and inputs are at least 36px high (WCAG 2.5.8 needs 24px).
- **Motion:** `prefers-reduced-motion: reduce` turns off animations, transitions and smooth
  scrolling.
- **Language:** `<html lang="de">`.
- **Status changes:** success messages use `role="status"`; every error message uses the shared
  error component with `role="alert"` (Owner 2026-10-22, DQ-5; `web/src/components/ErrorMessage.tsx`).
- **Decorative images:** class emblems and boss portraits next to a visible name use empty
  `alt` and `aria-hidden`; the guide and addon screenshots have German alt text.

## Interaction & Rendering Safety

- **Text only:** names, notes, poll answers, Rising-Gods data, and addon lines appear as plain
  text, never as formatted HTML (rule: SC-022).
- **Confirmations:** cancelling or archiving a raid, withdrawing loot, removing a roster row,
  merging accounts or characters, moving a character, and finishing or removing a Shadowmourne
  entry ask for confirmation first; declining
  sends nothing (ADR 069, ADR 076). The archive dialog shows the EP preview before it commits
  (FR-10.11).
- **Secrets shown once:** recovery codes, the TOTP setup secret and new registration codes are
  shown one time only (SC-013, SC-014), and the screen says so ("Wiederherstellungscodes werden
  nur jetzt angezeigt.").
- **MFA choice:** the login screen shows only the factors the server reported, security key first
  (FR-3.7, FR-3.8).
- **External links:** Rising-Gods profiles, Wowhead items and full-size screenshots open in a new
  tab with `rel="noreferrer"`.
- **External assets:** class icons, boss portraits and all other images come from the site
  itself (Owner 2026-10-22, DQ-2). Item icons in the loot catalog stay on `wow.zamimg.com`
  (Owner 2026-10-26, ADR 104); the content security policy is SC-024.
- **Visible role gating:** controls a role cannot use are hidden or explained; the server check
  is SC-001.

## Open Questions

None open. The owner resolved DQ-1 to DQ-5 on 2026-10-22; the answers are in the rules marked
"Owner 2026-10-22". IDs are not reused.

- **DQ-1** Control borders use `outline-control` `#5989a3`.
- **DQ-2** Class icons are served from the site. Amended 2026-10-26 (ADR 104): `wow.zamimg.com` stays in the CSP for item icons only.
- **DQ-3** No text below 12px.
- **DQ-4** New screens are designed in their implementation issues.
- **DQ-5** One shared error component with `role="alert"`.


### Raid-News (Owner 2026-10-28)
- Dashboard news use existing cards and German labels. Important unread messages display a labelled dialog with “Verstanden”.
- News rendered as plain text respecting line breaks. Staff editing behind a disclosure.
