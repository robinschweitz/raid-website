# 118 — Embed the gear planner in character profiles

Amends ADR 117 after the user preview. Keep viewer isolation using a same-origin frame, but let the website own navigation, character selection, and authenticated wishlist writes. Use existing profile IDs rather than locally invented characters. An item display ID represents an appearance, never a wishlist item ID.
