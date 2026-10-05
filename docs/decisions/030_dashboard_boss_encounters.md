# ADR 030: Instance-specific dashboard boss overview

Status: Accepted

The dashboard shows the configured raid instance's encounters with bundled Blizzard
achievement icons and Encounter Journal portraits instead of numbered placeholders.
Multi-NPC fights count as one encounter. Optional bosses are included; loot-only
chests and non-boss NPCs are excluded. Counts: Naxxramas 15, Ulduar 14, Vault of
Archavon 4, Icecrown Citadel 12, Trial of the Crusader 5, Ruby Sanctum 4 (including
its three lieutenants).

This is display metadata, not a replacement for the authoritative loot catalog.
Unknown instance IDs show an explicit unavailable state. No kill tracking exists,
so the overview does not invent a defeated count or current boss.
