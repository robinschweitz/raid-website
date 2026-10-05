# ADR 036: Class emblems and combat groups in the raid roster

## Decision

The owner requested a distinct graphic for each class and approved grouping damage
dealers by class and the specialization saved in the raid attendance snapshot.

The roster displays Tanks, Heilung, Nahkampf, and Fernkampf. The stored TANK/HEAL
role takes precedence. Damage-dealing death knights, warriors, paladins, and rogues
are melee; hunters, mages, priests, and warlocks are ranged. Druids use Wilder Kampf
(Cat) for melee and Gleichgewicht (Eule) for ranged; shamans use Verstärkung for
melee and Elementar for ranged.

Ambiguous historical class/spec snapshots remain visible in Schaden · unklar rather
than receiving a guessed combat group. Bench participants remain in their own
section with their derived group label. These are presentation groups, not new
persisted attendance roles. Live character changes never alter this classification.

Ten original, bundled SVG emblems identify the classes alongside readable class
names and class colors. Unknown classes receive a neutral fallback emblem. The
responsive roster uses four columns on wide screens, two on medium screens, and
one on mobile. Existing inline editing and permission checks remain applicable.
