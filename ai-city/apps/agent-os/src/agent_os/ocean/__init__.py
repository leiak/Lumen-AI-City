"""OCEAN personality → emotion baseline mapping.

Converts an OceanPersonality (5-dim OCEAN vector) into an EmotionDistribution
(8-emotion weights) via linear-additive coefficients. Pure math, no I/O.

Used by NpcTemplate loader to cache per-NPC baseline, which dispatcher
injects into prompt as 【人格基线情绪】 section.
"""