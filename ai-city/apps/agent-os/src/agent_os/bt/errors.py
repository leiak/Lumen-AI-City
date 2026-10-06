"""BT runtime errors."""
from __future__ import annotations


class BTError(Exception):
    """Raised on BT runtime violations (max_ticks, missing subtree, ...)."""