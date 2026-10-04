# ADR 026: Icy Guild Product Design

## Decision
The owner supplied an ICC-centered concept board for Bratwurst des Schicksals and approved
original SVG/CSS artwork. Use deep navy/almost-black backgrounds, icy cyan/blue and silver
accents, restrained frost borders and glow, readable modern data typography, and original
crest/citadel motifs. Avoid gold-heavy styling, casino presentation, and decorative clutter.

Desktop uses a branded sidebar and a prominent next-raid banner. Mobile uses compact cards
and companion navigation. Real roster snapshots drive class-colored character presentation.
EP cards show transparent balances, shifted range previews, and history without roll/spend
controls. Raid staff records outside loot outcomes in a separate permission-checked workspace.

Do not reproduce a screenshot as the application background. Do not fabricate boss progression,
RSVP percentages, notifications, performance statistics, or dead navigation entries. Supported
raid/roster/loot/EP/poll data is presented first; additional metrics need authoritative workflows.
Keep the existing React/hash-router/fetch stack and add no dependencies without owner approval.
