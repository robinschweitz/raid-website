# ADR 030: Focused secondary-page workspaces

## Decision

The owner requested that secondary pages follow the supplied concept art more closely,
especially the compact “Raid Setup – This Week” panel, while retaining the dashboard design.

The raid detail page opens on a role-grouped roster below a compact instance banner.
Tank, healer, and damage columns use stored attendance snapshots; bench participants appear
separately and substitutes retain their role with a badge. Counts reflect actual records,
not invented target compositions. Melee and ranged are not inferred from free-text specs.

Roster, loot, and planning/details have separate selectable views. Existing forms remain
mounted to preserve drafts when switching views. Creation forms are disclosed on demand;
correction actions open their corresponding editor. Catalog provenance is secondary detail.
Archived and cancelled permissions continue to follow the existing server rules.

Player profiles separate EP, loot, characters, locks, and administrator account controls. Armor tokens are shown by filtering profile loot.
Directories emphasize existing records ahead of creation forms. Configuration and poll
panels use consistent spacing, compact headings, and responsive grids. Styling is scoped
to secondary pages. No dependency or business-rule change is introduced.
