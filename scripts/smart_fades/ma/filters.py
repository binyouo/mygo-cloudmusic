"""MA schedule band identifiers; native filters live in internal/player/ma_render.go.

Adapted from Music Assistant e16a468, Apache-2.0; see SOURCE.json.
"""
from enum import StrEnum

ECHO_DECAYS = (0.5, 0.25, 0.12, 0.06)


class ShelfType(StrEnum):
    LOW = "lowshelf"
    HIGH = "highshelf"
    PEAK = "equalizer"
